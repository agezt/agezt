// SPDX-License-Identifier: MIT

// Memory retrieval + lifecycle: Recall/RecallScoped, Forget, Promote, Hygiene, Prune, Supersede, Get/Active/All/Count, Search/SearchScoped.
// Code extracted from manager.go during the Day-44 god-file split. Public API unchanged.
package memory


import (
	"context"

	"github.com/agezt/agezt/kernel/event"
)


// Recall ranks usable records against query and journals a memory.retrieved
// event (under corr) when anything matched, so `agt why` shows exactly what
// knowledge was surfaced for a task. Returns the ranked results (possibly
// empty).
func (m *Manager) Recall(corr, query string, limit int) ([]Scored, error) {
	return m.RecallScoped(corr, query, limit, "")
}

// RecallScoped is Recall restricted to a caller's visibility: shared records
// (no scope tag) are ALWAYS surfaced; a record private to some scope is surfaced
// only when that same scope is requested. An empty scope therefore sees shared
// memory only — which is what the daemon's automatic pre-run recall uses, so a
// run never inherits another agent's private notes (M652). This is the per-agent
// layer over the one shared brain: agents share most knowledge but can keep some
// notes to themselves by naming a scope.
func (m *Manager) RecallScoped(corr, query string, limit int, scope string) ([]Scored, error) {
	all, err := m.store.All()
	if err != nil {
		return nil, err
	}
	all = filterScope(all, scope)
	nowMS := m.now().UnixMilli()
	// Hybrid (M803): exact-keyword precision + local-embedding cosine for
	// typo/morphology recall — a misspelled or inflected query still
	// surfaces the right record.
	hits := SearchHybrid(all, query, limit, nowMS)
	engine := "local"
	// Provider embeddings opt-in (M884): when an Embedder is installed, true
	// semantic similarity replaces the feature-hash signal — keyword precision
	// stays blended in. Any embedder failure falls back to the local hits
	// already computed above; recall never fails because the embedder did.
	if emb := m.getEmbedder(); emb != nil {
		ctx, cancel := context.WithTimeout(context.Background(), embedTimeout)
		if sem, err := m.semanticProvider(ctx, emb, all, query, nowMS); err == nil {
			hits = mergeScored(Search(all, query, 0, nowMS), sem, limit)
			engine = "provider"
		}
		cancel()
	}
	if len(hits) > 0 {
		ids := make([]string, 0, len(hits))
		for _, h := range hits {
			ids = append(ids, h.Record.ID)
		}
		m.publish(event.KindMemoryRetrieved, corr, map[string]any{
			"query":    query,
			"matched":  len(hits),
			"ids":      ids,
			"embedder": engine,
		})
	}
	return hits, nil
}

// Forget soft-deletes a record (tombstone) and journals it. The record stays
// on disk and in the journal — recall excludes it, but it can be recovered
// and the action is auditable/reversible. Returns false if id is unknown.
func (m *Manager) Forget(corr, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, found, err := m.store.Get(id)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	if rec.Tombstoned {
		return true, nil // already forgotten; idempotent
	}
	rec.Tombstoned = true
	rec.LastSeenMS = m.now().UnixMilli()
	if err := m.store.Put(rec); err != nil {
		return false, err
	}
	m.publish(event.KindMemoryForgotten, corr, map[string]any{
		"id":      id,
		"subject": rec.Subject,
	})
	return true, nil
}

// Promote shares a private record (M915): its scope tag is cleared so the
// record joins the shared brain every agent recalls. This is the selective-
// sharing valve: agents accumulate private notes by default, and only the few
// worth everyone knowing are promoted (by the operator, or a future policy).
// Idempotent — promoting an already-shared record reports found without a
// write. The record keeps its id: identity is stable once created, and a later
// identical shared write would simply create a sibling record consolidation
// merges away. Returns false if id is unknown.
func (m *Manager) Promote(corr, id string) (Record, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, found, err := m.store.Get(id)
	if err != nil || !found {
		return Record{}, found, err
	}
	scope := scopeOf(rec.Tags)
	if scope == "" {
		return rec, true, nil // already shared
	}
	delete(rec.Tags, "scope")
	rec.LastSeenMS = m.now().UnixMilli()
	if err := m.store.Put(rec); err != nil {
		return Record{}, true, err
	}
	m.publish(event.KindMemoryPromoted, corr, map[string]any{
		"id":         id,
		"subject":    rec.Subject,
		"from_scope": scope,
	})
	return rec, true, nil
}

// HygieneStats summarizes the store's health for the maintenance view (M857).
type HygieneStats struct {
	Total      int `json:"total"`
	Active     int `json:"active"`
	Tombstoned int `json:"tombstoned"`
	Superseded int `json:"superseded"`
	Suspended  int `json:"suspended"`
	Expired    int `json:"expired"`
	// Prunable is how many soft-deleted (tombstoned or superseded) records are
	// older than the given cutoff — the dead weight a Prune would reclaim.
	Prunable int `json:"prunable"`
}

// Hygiene reports store health, counting how many soft-deleted records are older
// than olderThanMs (the prune candidates). olderThanMs <= 0 counts all
// soft-deleted records as prunable.
func (m *Manager) Hygiene(olderThanMs int64) (HygieneStats, error) {
	all, err := m.store.All()
	if err != nil {
		return HygieneStats{}, err
	}
	var st HygieneStats
	st.Total = len(all)
	for _, r := range all {
		switch {
		case r.Tombstoned:
			st.Tombstoned++
		case r.SupersededBy != "":
			st.Superseded++
		case r.Suspended():
			st.Suspended++
		case r.Expired(m.now().UnixMilli()):
			st.Expired++
		default:
			st.Active++
		}
		if (r.Tombstoned || r.SupersededBy != "") && (olderThanMs <= 0 || r.LastSeenMS < olderThanMs) {
			st.Prunable++
		}
	}
	return st, nil
}

// Prune hard-removes soft-deleted records (tombstoned or superseded) whose last
// activity predates olderThanMs — reclaiming the dead weight that consolidation
// and forgets leave behind, so memory can't grow without bound ("no memory-bomb",
// M857). Active records are never touched. With dryRun, nothing is deleted and
// the count of candidates is returned. The deletion is journaled (one
// memory.pruned event) and is the ONLY destructive memory op — by construction it
// only removes records already soft-deleted and aged out.
func (m *Manager) Prune(corr string, olderThanMs int64, dryRun bool) (int, error) {
	all, err := m.store.All()
	if err != nil {
		return 0, err
	}
	var victims []string
	for _, r := range all {
		if (r.Tombstoned || r.SupersededBy != "") && r.LastSeenMS < olderThanMs {
			victims = append(victims, r.ID)
		}
	}
	if dryRun || len(victims) == 0 {
		return len(victims), nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	pruned := 0
	for _, id := range victims {
		if ok, derr := m.store.Delete(id); derr != nil {
			return pruned, derr
		} else if ok {
			pruned++
		}
	}
	m.publish(event.KindMemoryPruned, corr, map[string]any{"pruned": pruned})
	return pruned, nil
}

// Supersede replaces an existing record with a new one, linking the old
// record's SupersededBy to the new id (soft update — the old record is
// retained). Returns the new record. If oldID is unknown, the new record is
// still created (supersession of nothing is just a create).
func (m *Manager) Supersede(corr, oldID string, spec RememberSpec) (Record, error) {
	newRec, _, err := m.Remember(corr, spec)
	if err != nil {
		return Record{}, err
	}
	// Remember already released the lock; re-take it for the old-record Get→Put.
	// Distinct critical section (different id), so no re-entrancy.
	m.mu.Lock()
	defer m.mu.Unlock()
	old, found, err := m.store.Get(oldID)
	if err != nil {
		return Record{}, err
	}
	if found && old.ID != newRec.ID {
		old.SupersededBy = newRec.ID
		old.LastSeenMS = m.now().UnixMilli()
		if err := m.store.Put(old); err != nil {
			return Record{}, err
		}
		m.publish(event.KindMemorySuperseded, corr, map[string]any{
			"old_id": oldID,
			"new_id": newRec.ID,
		})
	}
	return newRec, nil
}

// Get returns a single record by id (any state). Used by the control plane's
// `agt memory get`.
func (m *Manager) Get(id string) (Record, bool, error) { return m.store.Get(id) }

// Active returns every usable record, sorted deterministically. Used by
// `agt memory list` and as the recall corpus.
func (m *Manager) Active() ([]Record, error) {
	all, err := m.store.All()
	if err != nil {
		return nil, err
	}
	nowMS := m.now().UnixMilli()
	out := all[:0]
	for _, r := range all {
		if r.Usable(nowMS) {
			out = append(out, r)
		}
	}
	return out, nil
}

// All returns every record including tombstoned/superseded ones.
func (m *Manager) All() ([]Record, error) { return m.store.All() }

// Count returns the total number of stored records (all states). Used by
// `agt status`.
func (m *Manager) Count() int { return m.store.Count() }

// Search ranks active records against query without journaling — used by the
// control plane's `agt memory search`, which is a read operation an operator
// runs ad hoc (Recall is the run-time path that journals provenance).
func (m *Manager) Search(query string, limit int) ([]Scored, error) {
	all, err := m.store.All()
	if err != nil {
		return nil, err
	}
	return SearchHybrid(all, query, limit, m.now().UnixMilli()), nil
}

// SearchScoped ranks visible records without journaling. It is the quiet
// candidate-list path for context selection manifests: runtime can explain both
// chosen and rejected memory candidates without emitting a second
// memory.retrieved event or changing the actual recall behaviour.
func (m *Manager) SearchScoped(query string, limit int, scope string) ([]Scored, error) {
	all, err := m.store.All()
	if err != nil {
		return nil, err
	}
	all = filterScope(all, scope)
	return SearchHybrid(all, query, limit, m.now().UnixMilli()), nil
}

// Suspend marks a record as retained-but-not-usable. This is the operational
// form of "competitive suppression": no destructive edit, no active recall.