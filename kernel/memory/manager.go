// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
)

// Manager wraps a Store with the kernel bus so every mutation is journaled
// (durable-before-publish) and carries the originating run's correlation_id.
// This is the journaling boundary — the Store itself stays pure. It mirrors
// how kernel/runtime publishes kernel.halt/resume rather than the state store
// doing its own journaling.
type Manager struct {
	store Store
	bus   *bus.Bus
	// now is the clock, injectable for deterministic tests. Defaults to
	// time.Now when constructed via NewManager.
	now func() time.Time
	// mu serialises the read-modify-write mutators (Remember/Forget/Supersede) so
	// two concurrent writers — e.g. the agent loop and the auto-distiller both
	// remembering a fact, or a reinforce racing a forget — cannot interleave their
	// Get→Put pairs and lose an update (M421). The underlying Store guards each call
	// individually but not the pair.
	mu sync.Mutex
	// embMu guards the optional provider embedder + its vector cache (M884).
	// Separate from mu: embedding work must not block writers.
	embMu    sync.Mutex
	embedder Embedder
	embCache map[string][]float32 // record ID → vector; content-addressing makes it immutable
}

// NewManager wires a Store to a bus. bus may be nil in tests that only
// exercise the store-facing behaviour, but production callers always pass the
// kernel bus so mutations are auditable.
func NewManager(store Store, b *bus.Bus) *Manager {
	return &Manager{store: store, bus: b, now: time.Now}
}

// RememberSpec is the input to Remember.
type RememberSpec struct {
	Type       Type
	Subject    string
	Content    string
	Tags       map[string]string
	Confidence float64
	Evidence   Evidence
	HalfLifeMS int64
	// Actor records WHO is writing (M851): the acting agent's slug, or
	// "operator"/"distill" for non-agent writes. Stored as AddedBy (first writer)
	// and UpdatedBy (latest writer) on the record. Empty leaves provenance unset.
	Actor string
	// Force bypasses long-term retention filtering. Operator/control-plane writes
	// set this because they are explicit curation; automatic and agent writes do
	// not.
	Force bool
}

// Remember stores (or reinforces) a memory record and journals the write.
// Content-addressing means an identical (type, subject, content) triple
// dedupes onto the existing record: rather than creating a duplicate, the
// existing record's recency is refreshed and its confidence nudged up
// ("re-observed"). A previously tombstoned record is revived by a fresh
// Remember. Returns the record and whether it was newly created.
func (m *Manager) Remember(corr string, spec RememberSpec) (Record, bool, error) {
	if strings.TrimSpace(spec.Content) == "" {
		return Record{}, false, ErrEmptyContent
	}
	t := spec.Type
	if t == "" {
		t = DefaultType
	}
	if !ValidType(t) {
		return Record{}, false, fmt.Errorf("memory: invalid type %q", t)
	}
	conf := spec.Confidence
	if conf <= 0 {
		conf = 1.0
	}
	if conf > 1 {
		conf = 1
	}
	if !spec.Force && shouldFilterSpec(spec) {
		if d := assessSpec(spec); !d.Keep {
			return Record{}, false, fmt.Errorf("memory: low-value record rejected (%s)", d.Reason)
		}
	}
	evidence := normalizeEvidence(spec.Evidence, spec.Tags, t)
	halfLifeMS := spec.HalfLifeMS
	if halfLifeMS <= 0 {
		halfLifeMS = defaultHalfLifeMS(t, evidence)
	}
	nowMS := m.now().UnixMilli()
	// Scope participates in identity (M915): two agents privately noting the
	// same content get two records, instead of the second write reinforcing the
	// first and flipping its scope tag (which would hide the note from its
	// original author). Shared writes hash exactly as before.
	id := ScopedID(t, spec.Subject, spec.Content, scopeOf(spec.Tags))

	// Hold the lock across the Get→Put so a concurrent writer can't lose the update.
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, found, err := m.store.Get(id)
	if err != nil {
		return Record{}, false, err
	}

	rec := Record{
		ID:         id,
		Type:       t,
		Subject:    spec.Subject,
		Content:    spec.Content,
		Tags:       spec.Tags,
		Confidence: conf,
		Evidence:   evidence,
		HalfLifeMS: halfLifeMS,
		CreatedMS:  nowMS,
		LastSeenMS: nowMS,
		AddedBy:    spec.Actor,
		UpdatedBy:  spec.Actor,
	}
	action := "create"
	if found {
		// Reinforce: keep original creation time, refresh recency, and
		// strengthen confidence toward 1.0. Revive if it was tombstoned.
		rec.CreatedMS = existing.CreatedMS
		rec.SourceEvent = existing.SourceEvent
		rec.Confidence = clampConf(existing.Confidence + 0.1)
		if spec.Evidence == "" {
			rec.Evidence = existing.Evidence
		}
		if spec.HalfLifeMS <= 0 {
			rec.HalfLifeMS = existing.HalfLifeMS
		}
		// Provenance: AddedBy is first-writer-wins (preserve the original author,
		// like SourceEvent); UpdatedBy reflects this latest write. A legacy record
		// with no AddedBy adopts this writer as its author.
		if existing.AddedBy != "" {
			rec.AddedBy = existing.AddedBy
		}
		if spec.Actor == "" {
			rec.UpdatedBy = existing.UpdatedBy
		}
		if existing.Tags != nil && rec.Tags == nil {
			rec.Tags = existing.Tags
		}
		// A successful re-observation/reconstruction makes a suspended record
		// usable again without erasing its audit history from the journal.
		rec.SuspendedMS = 0
		rec.SuspendedReason = ""
		// Preserve a supersession link: re-stating content that was explicitly
		// superseded must NOT silently resurrect it as active (it has a designated
		// successor). Without this, rec.SupersededBy="" would overwrite the link and
		// Active()/Recall would return the stale fact alongside its replacement.
		rec.SupersededBy = existing.SupersededBy
		action = "reinforce"
		if existing.Tombstoned {
			action = "revive"
		}
	}

	// Publish first so the event id can be recorded as provenance, then
	// persist. A crash between the two leaves an orphan audit event and no
	// stored record — harmless, since the store is the retrieval source of
	// truth and the journal is append-only audit.
	payload := map[string]any{
		"action":     action,
		"id":         id,
		"type":       string(t),
		"subject":    spec.Subject,
		"chars":      len(spec.Content),
		"confidence": rec.Confidence,
	}
	if rec.Evidence != "" {
		payload["evidence"] = string(rec.Evidence)
	}
	if rec.HalfLifeMS > 0 {
		payload["half_life_ms"] = rec.HalfLifeMS
	}
	if rec.UpdatedBy != "" {
		payload["actor"] = rec.UpdatedBy // who wrote this (M851)
	}
	ev := m.publish(event.KindMemoryWritten, corr, payload)
	if ev != nil && rec.SourceEvent == "" {
		rec.SourceEvent = ev.ID
	}
	if err := m.store.Put(rec); err != nil {
		return Record{}, false, err
	}
	return rec, !found, nil
}

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
func (m *Manager) Suspend(corr, id, reason string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, found, err := m.store.Get(id)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	nowMS := m.now().UnixMilli()
	rec.SuspendedMS = nowMS
	rec.SuspendedReason = strings.TrimSpace(reason)
	rec.LastSeenMS = nowMS
	if err := m.store.Put(rec); err != nil {
		return false, err
	}
	m.publish(event.KindMemorySuspended, corr, map[string]any{
		"id":      id,
		"subject": rec.Subject,
		"reason":  rec.SuspendedReason,
	})
	return true, nil
}

// AuditReport is the memory hygiene view used to detect stale or competing
// memories before they contaminate retrieval.
type AuditReport struct {
	Total             int                  `json:"total"`
	Usable            int                  `json:"usable"`
	Suspended         int                  `json:"suspended"`
	Expired           int                  `json:"expired"`
	ContradictionLoad int                  `json:"contradiction_load"`
	ExpiredIDs        []string             `json:"expired_ids,omitempty"`
	SuspendedIDs      []string             `json:"suspended_ids,omitempty"`
	Contradictions    []ContradictionGroup `json:"contradictions,omitempty"`
}

// CleanReport summarizes low-value retention cleanup. Removed means hard-deleted:
// clean targets records that never belonged in memory, not records worth a
// reversible tombstone.
type CleanReport struct {
	DryRun      bool               `json:"dry_run"`
	HardDeleted bool               `json:"hard_deleted"`
	Scanned     int                `json:"scanned"`
	Rejected    int                `json:"rejected"`
	Removed     int                `json:"removed"`
	Decisions   []CleanDecisionRow `json:"decisions,omitempty"`
}

type CleanDecisionRow struct {
	ID      string `json:"id"`
	Subject string `json:"subject,omitempty"`
	Reason  string `json:"reason"`
}

type ContradictionGroup struct {
	Key     string   `json:"key"`
	IDs     []string `json:"ids"`
	Subject string   `json:"subject,omitempty"`
	Type    Type     `json:"type,omitempty"`
	Scope   string   `json:"scope,omitempty"`
}

// Audit finds records that are barred by expiration/suspension and same-topic
// active records that compete with different content. It does not claim which
// record is true; it only exposes the contradiction load.
func (m *Manager) Audit() (AuditReport, error) {
	all, err := m.store.All()
	if err != nil {
		return AuditReport{}, err
	}
	nowMS := m.now().UnixMilli()
	report := AuditReport{Total: len(all)}
	byKey := map[string][]Record{}
	for _, r := range all {
		if !r.Active() {
			continue
		}
		switch {
		case r.Suspended():
			report.Suspended++
			report.SuspendedIDs = append(report.SuspendedIDs, r.ID)
			continue
		case r.Expired(nowMS):
			report.Expired++
			report.ExpiredIDs = append(report.ExpiredIDs, r.ID)
			continue
		default:
			report.Usable++
		}
		if contradictionTrackedType(r.Type) {
			byKey[contradictionKey(r)] = append(byKey[contradictionKey(r)], r)
		}
	}
	for key, rs := range byKey {
		if len(rs) < 2 || sameNormalizedContent(rs) {
			continue
		}
		ids := make([]string, 0, len(rs))
		for _, r := range rs {
			ids = append(ids, r.ID)
		}
		report.Contradictions = append(report.Contradictions, ContradictionGroup{
			Key:     key,
			IDs:     ids,
			Subject: rs[0].Subject,
			Type:    rs[0].Type,
			Scope:   scopeOf(rs[0].Tags),
		})
		report.ContradictionLoad += len(rs) - 1
	}
	return report, nil
}

// CleanLowValue applies the retention filter to the store and hard-deletes
// records that do not belong in memory at all: execution logs, transient sweep
// notes, and automatic low-value records. This is deliberately stricter than
// Forget/Prune: clean is the "this was never memory" path. Dry-run is the
// default at the control-plane/CLI layer; execute mode reclaims the rows
// immediately.
func (m *Manager) CleanLowValue(corr string, dryRun bool) (CleanReport, error) {
	all, err := m.store.All()
	if err != nil {
		return CleanReport{}, err
	}
	report := CleanReport{DryRun: dryRun, HardDeleted: !dryRun, Scanned: len(all)}
	var victims []string
	for _, r := range all {
		if sourceOf(r.Tags) == "operator" || r.AddedBy == "operator" || r.UpdatedBy == "operator" ||
			r.Evidence == EvidenceCurated || r.Evidence == EvidenceConstraint || r.Type == TypePreference {
			continue
		}
		decision := AssessRetention(r)
		if decision.Keep {
			continue
		}
		report.Rejected++
		report.Decisions = append(report.Decisions, CleanDecisionRow{ID: r.ID, Subject: r.Subject, Reason: decision.Reason})
		if dryRun {
			continue
		}
		victims = append(victims, r.ID)
	}
	if !dryRun && len(victims) > 0 {
		m.mu.Lock()
		for _, id := range victims {
			ok, derr := m.store.Delete(id)
			if derr != nil {
				m.mu.Unlock()
				return report, derr
			}
			if ok {
				report.Removed++
			}
		}
		m.mu.Unlock()
	}
	if !dryRun && report.Removed > 0 {
		m.publish(event.KindMemoryCleaned, corr, map[string]any{
			"scanned":      report.Scanned,
			"rejected":     report.Rejected,
			"removed":      report.Removed,
			"hard_deleted": true,
		})
	}
	return report, nil
}

// publish writes one event through the bus, returning the persisted event (or
// nil when no bus is wired, e.g. store-only tests). Subject groups memory
// events under "memory.<suffix>" so subscribers can scope-filter.
func (m *Manager) publish(kind event.Kind, corr string, payload any) *event.Event {
	if m.bus == nil {
		return nil
	}
	suffix := strings.TrimPrefix(string(kind), "memory.")
	ev, _ := m.bus.Publish(event.Spec{
		Subject:       "memory." + suffix,
		Kind:          kind,
		Actor:         "memory",
		CorrelationID: corr,
		Payload:       payload,
	})
	return ev
}

func clampConf(c float64) float64 {
	if c < 0 {
		return 0
	}
	if c > 1 {
		return 1
	}
	return c
}

// --- run-time context plumbing -------------------------------------------

type ctxKey int

const ctxKeyCorrelation ctxKey = iota

// WithCorrelation returns a child context carrying corr so the in-process
// memory Tool can journal its writes under the originating run. The runtime
// sets this on every run's context; without it the tool falls back to an
// empty correlation (still journaled, just not linked).
func WithCorrelation(ctx context.Context, corr string) context.Context {
	return context.WithValue(ctx, ctxKeyCorrelation, corr)
}

// CorrelationFrom extracts the correlation id set by WithCorrelation.
func CorrelationFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyCorrelation).(string); ok {
		return v
	}
	return ""
}

const ctxKeyScope ctxKey = iota + 1

// WithScope returns a child context carrying the run's per-agent memory scope
// (M786): when a run executes AS a named agent (M783), its recalls — the
// context injection and the memory tool — default to this scope, so the agent
// sees its own private notes on top of shared memory without having to name
// itself. Writes default to this scope too (M915 — each agent keeps its own
// memory; the shared brain is opt-in via the tool's shared=true and kept
// selective). The explicit tool scope param always wins over this default.
func WithScope(ctx context.Context, scope string) context.Context {
	if scope == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyScope, scope)
}

// ScopeFrom extracts the per-agent memory scope set by WithScope ("" = none).
func ScopeFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyScope).(string); ok {
		return v
	}
	return ""
}

// --- agent tool -----------------------------------------------------------

// toolInputSchema is the JSON Schema advertised to the model for the
// in-process `memory` tool.
const toolInputSchema = `{
  "type": "object",
  "properties": {
    "action":  {"type": "string", "enum": ["remember", "recall", "forget", "find_related", "bulk_forget"], "description": "what to do"},
    "subject": {"type": "string", "description": "entity/topic the memory is about (remember)"},
    "content": {"type": "string", "description": "the text to remember (remember)"},
    "type":    {"type": "string", "enum": ["FACT","SUMMARY","RELATION","PREFERENCE","OBSERVATION"], "description": "memory type (remember; default FACT)"},
    "evidence":{"type": "string", "enum": ["observed","inferred","curated","constraint"], "description": "epistemic source class (remember; default inferred/derived from source)"},
    "half_life_ms":{"type": "integer", "description": "mechanical expiration budget in milliseconds (remember; default by evidence/type)"},
    "query":   {"type": "string", "description": "search text (recall)"},
    "limit":   {"type": "integer", "description": "max results (recall; default 5)"},
    "id":      {"type": "string", "description": "record id (forget, find_related)"},
    "ids":     {"type": "array", "items": {"type": "string"}, "description": "record ids (bulk_forget)"},
    "shared":  {"type": "boolean", "description": "remember only: write to the SHARED memory every agent recalls. Be selective — share only durable facts useful to ALL agents (owner preferences, project-wide decisions). Default false: the note stays private to you."},
    "scope":   {"type": "string", "description": "optional namespace override, e.g. a role like \"researcher\". On remember: store the note private to that scope (default: your own agent scope). On recall: also surface that scope's private notes. Shared memory is ALWAYS visible; another scope's private notes never are."}
  },
  "required": ["action"]
}`

type toolInput struct {
	Action     string   `json:"action"`
	Subject    string   `json:"subject"`
	Content    string   `json:"content"`
	Type       Type     `json:"type"`
	Evidence   Evidence `json:"evidence"`
	HalfLifeMS int64    `json:"half_life_ms"`
	Query      string   `json:"query"`
	Limit      int      `json:"limit"`
	ID         string   `json:"id"`
	IDs        []string `json:"ids"`
	Shared     bool     `json:"shared"`
	Scope      string   `json:"scope"`
}

// memoryTool is the in-process agent.Tool that lets the agent remember,
// recall, and forget during a run. Writes are journaled by the Manager under
// the run's correlation (read from ctx via CorrelationFrom).
type memoryTool struct{ mgr *Manager }

// Tool returns the agent-facing memory tool. Register it under the name
// "memory" in the agent loop's tool map.
func (m *Manager) Tool() agent.Tool { return memoryTool{mgr: m} }

func (t memoryTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "memory",
		Description: "Persist and retrieve durable knowledge across tasks. " +
			"action=remember stores a fact (subject, content); " +
			"action=recall searches stored memory (query); " +
			"action=forget tombstones a record (id). " +
			"Your notes are PRIVATE to you by default — recall surfaces them plus the shared memory. " +
			"Pass shared=true only for facts genuinely useful to ALL agents " +
			"(owner preferences, project-wide decisions); be selective about the shared brain.",
		Effect: agent.ToolEffect{
			Class: agent.EffectReversible,
			PredictedEffects: []string{
				"read durable memory for recall/find_related actions",
				"write or tombstone durable memory for remember/forget/bulk_forget actions",
			},
			AffectedResources: []string{"memory store", "agent/private memory scope", "shared memory scope when requested"},
			RollbackNotes:     "Recall needs no rollback. Remembered records can be tombstoned with forget; tombstones and writes are journaled for audit/replay.",
			Confidence:        0.9,
		},
		InputSchema: json.RawMessage(toolInputSchema),
	}
}

func (t memoryTool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid memory input: " + err.Error(), IsError: true}, nil
	}
	corr := CorrelationFrom(ctx)
	switch strings.ToLower(strings.TrimSpace(in.Action)) {
	case "remember":
		// Private-by-default (M915): a named agent's write lands in its OWN
		// scope unless it explicitly opts into the shared brain (shared=true,
		// or the "shared" scope sentinel a model may plausibly produce). An
		// explicit scope param still wins; an unscoped run (no agent identity)
		// keeps writing shared, as before.
		scope := strings.TrimSpace(in.Scope)
		switch {
		case in.Shared || strings.EqualFold(scope, "shared"):
			scope = ""
		case scope == "":
			scope = ScopeFrom(ctx)
		}
		rec, created, err := t.mgr.Remember(corr, RememberSpec{
			Type: in.Type, Subject: in.Subject, Content: in.Content, Tags: toolTags(scope),
			Evidence: in.Evidence, HalfLifeMS: in.HalfLifeMS,
			Actor: toolActor(ctx), // who is writing — the agent slug, or "agent" (M851)
		})
		if err != nil {
			return agent.Result{Output: "remember failed: " + err.Error(), IsError: true}, nil
		}
		verb := "reinforced"
		if created {
			verb = "stored"
		}
		where := "shared"
		if scope != "" {
			where = "private to " + scope
		}
		return agent.Result{Output: fmt.Sprintf("%s memory %s (%s: %s) — %s", verb, rec.ID[:12], rec.Type, rec.Subject, where)}, nil
	case "recall":
		limit := in.Limit
		if limit <= 0 {
			limit = 5
		}
		// The run's per-agent scope (M786) is the DEFAULT visibility: a named
		// agent recalls its own private notes + shared memory without naming
		// itself. An explicit scope param wins (e.g. peeking at a teammate's
		// scope is still expressible — records stay readable, never hidden
		// behind identity).
		scope := strings.TrimSpace(in.Scope)
		if scope == "" {
			scope = ScopeFrom(ctx)
		}
		hits, err := t.mgr.RecallScoped(corr, in.Query, limit, scope)
		if err != nil {
			return agent.Result{Output: "recall failed: " + err.Error(), IsError: true}, nil
		}
		return agent.Result{Output: renderHits(hits)}, nil
	case "forget":
		ok, err := t.mgr.Forget(corr, in.ID)
		if err != nil {
			return agent.Result{Output: "forget failed: " + err.Error(), IsError: true}, nil
		}
		if !ok {
			return agent.Result{Output: "no memory with id " + in.ID, IsError: true}, nil
		}
		return agent.Result{Output: "forgot memory " + in.ID}, nil
	case "find_related":
		if in.ID == "" {
			return agent.Result{Output: "find_related requires id", IsError: true}, nil
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}
		if limit > 100 {
			limit = 100
		}
		seed, found, err := t.mgr.Get(in.ID)
		if err != nil {
			return agent.Result{Output: "find_related failed: " + err.Error(), IsError: true}, nil
		}
		if !found {
			return agent.Result{Output: "seed memory id " + in.ID + " not found", IsError: true}, nil
		}
		hits, err := t.mgr.Search(seed.Content, limit+1) // +1 because seed itself may appear
		if err != nil {
			return agent.Result{Output: "find_related failed: " + err.Error(), IsError: true}, nil
		}
		// Exclude the seed record from results.
		out := make([]string, 0, limit)
		for _, h := range hits {
			if h.Record.ID != in.ID {
				out = append(out, fmt.Sprintf("[%.3f] %s (%s: %s)", h.Score, h.Record.Subject, h.Record.ID[:12], h.Record.Type))
			}
			if len(out) >= limit {
				break
			}
		}
		if len(out) == 0 {
			return agent.Result{Output: "no related memories found for " + in.ID}, nil
		}
		return agent.Result{Output: "related memories for " + in.ID + ":\n" + strings.Join(out, "\n")}, nil
	case "bulk_forget":
		if len(in.IDs) == 0 {
			return agent.Result{Output: "bulk_forget requires ids", IsError: true}, nil
		}
		if len(in.IDs) > 500 {
			return agent.Result{Output: "bulk_forget exceeds 500 ids per call", IsError: true}, nil
		}
		var forgotten, notFound int
		for _, id := range in.IDs {
			ok, err := t.mgr.Forget(corr, id)
			if err != nil {
				return agent.Result{Output: "bulk_forget failed: " + err.Error(), IsError: true}, nil
			}
			if ok {
				forgotten++
			} else {
				notFound++
			}
		}
		return agent.Result{Output: fmt.Sprintf("forgotten: %d  not_found: %d", forgotten, notFound)}, nil
	default:
		return agent.Result{Output: "unknown action " + in.Action + " (remember|recall|forget|find_related|bulk_forget)", IsError: true}, nil
	}
}

// toolActor resolves who an agent's memory write should be attributed to (M851):
// the named roster agent's slug when the run executes AS one, else the generic
// "agent" (a default-identity run). Operator (console/CLI) and distilled writes
// set their own actor at their call sites.
func toolActor(ctx context.Context) string {
	if slug := agent.AgentFromContext(ctx); slug != "" {
		return slug
	}
	return "agent"
}

// toolTags builds a tool write's tag map. Tool writes are tagged source=agent so
// they are distinguishable from operator and distilled writes; a non-empty scope
// tag makes the note private to that namespace (recall only surfaces it when the
// same scope is requested) — the per-agent layer over shared memory (M652/M915).
func toolTags(scope string) map[string]string {
	t := map[string]string{"source": "agent"}
	if scope != "" {
		t["scope"] = scope
	}
	return t
}

// scopeOf extracts the scope tag from a record's tag map ("" = shared).
func scopeOf(tags map[string]string) string {
	if tags == nil {
		return ""
	}
	return tags["scope"]
}

// filterScope drops records private to a scope other than the requested one.
// A record is visible when it carries no scope tag (shared) or its scope equals
// the caller's. Returns a new slice; the input is not mutated.
func filterScope(recs []Record, scope string) []Record {
	out := make([]Record, 0, len(recs))
	for _, r := range recs {
		rs := ""
		if r.Tags != nil {
			rs = r.Tags["scope"]
		}
		if rs == "" || rs == scope {
			out = append(out, r)
		}
	}
	return out
}

func renderHits(hits []Scored) string {
	if len(hits) == 0 {
		return "no relevant memory found"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d relevant memor%s:\n", len(hits), plural(len(hits)))
	for _, h := range hits {
		scope := ""
		if h.Record.Tags != nil && h.Record.Tags["scope"] != "" {
			scope = " (scope: " + h.Record.Tags["scope"] + ")"
		}
		fmt.Fprintf(&b, "- [%s] %s: %s%s\n", h.Record.Type, h.Record.Subject, h.Record.Content, scope)
	}
	return strings.TrimRight(b.String(), "\n")
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// --- distillation ---------------------------------------------------------

// distillSystem instructs the provider to extract durable, reusable facts
// from a completed task. The model must return a JSON object so parsing is
// deterministic; any non-JSON or empty response yields zero facts (the
// best-effort contract — distillation never fails a task).
const distillSystem = `You review a completed agent task and extract durable, reusable facts worth remembering for future tasks. ` +
	`Return ONLY a JSON object of the form {"facts":[{"subject":"...","content":"...","type":"FACT|SUMMARY|PREFERENCE"}]}. ` +
	`Extract at most 3 facts. Prefer specific, durable knowledge (project structure, decisions, user preferences) over transient details. ` +
	`If nothing is worth remembering, return {"facts":[]}.`

type distillResult struct {
	Facts []struct {
		Subject string `json:"subject"`
		Content string `json:"content"`
		Type    Type   `json:"type"`
	} `json:"facts"`
}

// Distill runs one best-effort LLM call over a task transcript and stores any
// extracted facts (tagged source=distill) under corr. It returns the ids it
// created. Errors are returned for the caller to journal, but the caller must
// never let a distillation error fail the underlying task.
func (m *Manager) Distill(ctx context.Context, corr string, provider agent.Provider, model, intent, transcript string) ([]string, error) {
	if provider == nil {
		return nil, errors.New("memory: distill requires a provider")
	}
	user := fmt.Sprintf("Task intent:\n%s\n\nWhat happened:\n%s", intent, transcript)
	resp, err := provider.Complete(ctx, agent.CompletionRequest{
		Model:    model,
		System:   distillSystem,
		Messages: []agent.Message{{Role: agent.RoleUser, Content: user}},
		TaskType: "distill",
	})
	if err != nil {
		return nil, fmt.Errorf("memory: distill completion: %w", err)
	}
	parsed, ok := parseDistill(resp.Message.Content)
	if !ok {
		// Non-JSON answer (e.g. the mock provider) → nothing to store. Not
		// an error; distillation is opportunistic.
		return nil, nil
	}
	// A named agent's distilled facts stay its private notes (M915): the run
	// ctx carries the agent's scope, so per-run distillation doesn't flood the
	// shared brain. An unscoped run (operator chat) distills shared, as before.
	// Promotion or consolidation can share the keepers later.
	scope := ScopeFrom(ctx)
	tags := map[string]string{"source": "distill"}
	if scope != "" {
		tags["scope"] = scope
	}

	// Subject-level dedupe (M993): auto-distillation fires after most multi-tool
	// runs, so without this the SAME topic gets re-extracted run after run with
	// slightly reworded content — each a new content-hash, so they pile up into
	// thousands of near-duplicate notes ("her işlemde memory'ye bir şey ekliyor").
	// Index the existing active records by (type, normalized-subject, scope); when
	// a distilled fact lands on a subject we already hold in this scope, REINFORCE
	// the existing record (bump recency/confidence) instead of creating another.
	// New subjects are still recorded. The explicit `memory` tool is unaffected —
	// only opportunistic distillation is gated, so deliberate writes still stand.
	index := map[string]Record{}
	if active, err := m.Active(); err == nil {
		for _, r := range active {
			if r.Tags["source"] != "distill" {
				continue // only collapse onto prior distilled notes, not curated ones
			}
			index[distillKey(r.Type, r.Subject, scopeOf(r.Tags))] = r
		}
	}

	var ids []string
	for _, f := range parsed.Facts {
		if strings.TrimSpace(f.Content) == "" {
			continue
		}
		t := f.Type
		if !ValidType(t) {
			t = TypeSummary
		}
		key := distillKey(t, f.Subject, scope)
		if ex, ok := index[key]; ok {
			// Already noted this subject in this scope — reinforce the existing
			// record rather than adding a near-duplicate.
			if _, _, err := m.Remember(corr, RememberSpec{Type: ex.Type, Subject: ex.Subject, Content: ex.Content, Actor: "distill", Tags: ex.Tags}); err != nil {
				return ids, err
			}
			ids = append(ids, ex.ID)
			continue
		}
		rec, _, err := m.Remember(corr, RememberSpec{
			Type:    t,
			Subject: f.Subject,
			Content: f.Content,
			Actor:   "distill",
			Tags:    tags,
		})
		if err != nil {
			return ids, err
		}
		// Record it so two facts about the same new subject in one pass also collapse.
		index[key] = rec
		ids = append(ids, rec.ID)
	}
	return ids, nil
}

// distillKey is the subject-level identity used to collapse repeated
// auto-distilled notes: type + normalized subject + scope. Subject is lowercased
// and whitespace-collapsed so "Project structure" and "project  structure" map
// together.
func distillKey(t Type, subject, scope string) string {
	return string(t) + "\x00" + strings.Join(strings.Fields(strings.ToLower(subject)), " ") + "\x00" + scope
}

func normalizeEvidence(e Evidence, tags map[string]string, t Type) Evidence {
	switch e {
	case EvidenceObserved, EvidenceInferred, EvidenceCurated, EvidenceConstraint:
		return e
	}
	if tags != nil {
		switch tags["source"] {
		case "operator":
			return EvidenceCurated
		case "distill", "brain-distill", "agent":
			return EvidenceInferred
		}
	}
	if t == TypeObservation {
		return EvidenceObserved
	}
	return EvidenceInferred
}

func defaultHalfLifeMS(t Type, e Evidence) int64 {
	const dayMS = int64(24 * time.Hour / time.Millisecond)
	switch e {
	case EvidenceConstraint:
		return 3650 * dayMS
	case EvidenceCurated:
		return 180 * dayMS
	case EvidenceObserved:
		return 30 * dayMS
	}
	switch t {
	case TypePreference:
		return 180 * dayMS
	case TypeSummary:
		return 90 * dayMS
	case TypeObservation:
		return 30 * dayMS
	default:
		return 60 * dayMS
	}
}

func contradictionTrackedType(t Type) bool {
	switch t {
	case TypeFact, TypePreference, TypeRelation, TypeObservation:
		return true
	default:
		return false
	}
}

func contradictionKey(r Record) string {
	return string(r.Type) + "\x00" + normalizeTopic(r.Subject) + "\x00" + scopeOf(r.Tags)
}

func normalizeTopic(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

func normalizeContent(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

func sameNormalizedContent(rs []Record) bool {
	if len(rs) < 2 {
		return true
	}
	first := normalizeContent(rs[0].Content)
	for _, r := range rs[1:] {
		if normalizeContent(r.Content) != first {
			return false
		}
	}
	return true
}

// DedupeDistilled retroactively collapses the near-duplicate auto-distilled notes
// that accumulated before the write-time subject gate (M993) existed — the "1000+
// nonsense entries" the owner saw. It groups active source=distill records by
// distillKey and, for each group with more than one, keeps the strongest note
// (highest confidence, then most-recently-seen) and FORGETS the rest (soft
// tombstone — reversible, and prunable later). Curated memories (the explicit
// `memory` tool, source≠distill) are never touched. With dryRun it only reports
// how many would be collapsed. Returns the number removed (or that would be).
func (m *Manager) DedupeDistilled(corr string, dryRun bool) (int, error) {
	active, err := m.Active()
	if err != nil {
		return 0, err
	}
	groups := map[string][]Record{}
	for _, r := range active {
		if r.Tags["source"] != "distill" {
			continue
		}
		k := distillKey(r.Type, r.Subject, scopeOf(r.Tags))
		groups[k] = append(groups[k], r)
	}
	collapsed := 0
	for _, g := range groups {
		if len(g) < 2 {
			continue
		}
		keep := 0
		for i := 1; i < len(g); i++ {
			if strongerNote(g[i], g[keep]) {
				keep = i
			}
		}
		for i := range g {
			if i == keep {
				continue
			}
			if dryRun {
				collapsed++
				continue
			}
			ok, ferr := m.Forget(corr, g[i].ID)
			if ferr != nil {
				return collapsed, ferr
			}
			if ok {
				collapsed++
			}
		}
	}
	return collapsed, nil
}

// strongerNote ranks two same-subject distilled notes for which to keep: higher
// confidence wins (it was reinforced more), then the most-recently-seen.
func strongerNote(a, b Record) bool {
	if a.Confidence != b.Confidence {
		return a.Confidence > b.Confidence
	}
	return a.LastSeenMS > b.LastSeenMS
}

// parseDistill extracts the JSON object from a model response, tolerating
// surrounding prose or markdown fences by scanning for the outermost braces.
func parseDistill(s string) (distillResult, bool) {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return distillResult{}, false
	}
	var r distillResult
	if err := json.Unmarshal([]byte(s[start:end+1]), &r); err != nil {
		return distillResult{}, false
	}
	return r, true
}
