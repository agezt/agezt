// SPDX-License-Identifier: MIT

// World model query: resolveOrCreate + Resolve + ResolveQuiet + IsActiveSubject + Neighbors + Forget + Get + Entities + Relations + Count + publish + alias/attr helpers.
// Code extracted from manager.go during the Day-65 god-file split. Public API unchanged.
package worldmodel


import (
	"github.com/agezt/agezt/kernel/event"
	"maps"
	"sort"
	"strings"
)


func (g *Graph) resolveOrCreate(corr, name string) (string, error) {
	all, err := g.store.AllEntities()
	if err != nil {
		return "", err
	}
	folded := strings.ToLower(strings.TrimSpace(name))
	if folded == "" {
		return "", ErrEmptyName
	}
	for _, e := range all {
		if !e.Active() {
			continue
		}
		if strings.ToLower(strings.TrimSpace(e.Name)) == folded {
			return e.ID, nil
		}
		for _, a := range e.Aliases {
			if strings.ToLower(strings.TrimSpace(a)) == folded {
				return e.ID, nil
			}
		}
	}
	created, _, err := g.Upsert(corr, UpsertSpec{Kind: KindTopic, Name: name})
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

// Resolve ranks active entities against phrase and journals a
// worldmodel.retrieved event (under corr) when anything matched — so `agt why`
// shows what the system understood a reference to mean. Returns the ranked
// results (possibly empty).
func (g *Graph) Resolve(corr, phrase string, limit int) ([]ScoredEntity, error) {
	all, err := g.store.AllEntities()
	if err != nil {
		return nil, err
	}
	hits := Resolve(all, phrase, limit, g.now().UnixMilli())
	if len(hits) > 0 {
		ids := make([]string, 0, len(hits))
		for _, h := range hits {
			ids = append(ids, h.Entity.ID)
		}
		g.publish(event.KindWorldRetrieved, corr, map[string]any{
			"phrase":  phrase,
			"matched": len(hits),
			"ids":     ids,
		})
	}
	return hits, nil
}

// ResolveQuiet ranks active entities without journaling — used by ad-hoc
// operator queries (`agt world resolve`) and by IsActiveSubject, which must
// not write an event every time Pulse scores a delta.
func (g *Graph) ResolveQuiet(phrase string, limit int) ([]ScoredEntity, error) {
	all, err := g.store.AllEntities()
	if err != nil {
		return nil, err
	}
	return Resolve(all, phrase, limit, g.now().UnixMilli()), nil
}

// IsActiveSubject reports whether text refers to a known active entity, and if
// so returns that entity's name. This is the pulse.Relevance adapter (SPEC-05
// §3.4): "is this delta about a project the operator actually cares about?".
// A small score floor avoids a single incidental token counting as relevance.
func (g *Graph) IsActiveSubject(text string) (string, bool) {
	hits, err := g.ResolveQuiet(text, 1)
	if err != nil || len(hits) == 0 {
		return "", false
	}
	if hits[0].Score < 1.0 {
		return "", false
	}
	return hits[0].Entity.Name, true
}

// Neighbors returns the active edges incident to entityID with the adjacent
// entity for each.
func (g *Graph) Neighbors(entityID string) ([]Neighbor, error) {
	es, err := g.store.AllEntities()
	if err != nil {
		return nil, err
	}
	rs, err := g.store.AllRelations()
	if err != nil {
		return nil, err
	}
	return Neighbors(entityID, es, rs), nil
}

// Forget soft-deletes an entity or relation by id (tombstone) and journals it.
// The record stays on disk and in the journal — excluded from resolve/neighbors
// but recoverable and auditable. Returns false if id is unknown.
func (g *Graph) Forget(corr, id string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if e, found, err := g.store.GetEntity(id); err != nil {
		return false, err
	} else if found {
		if e.Tombstoned {
			return true, nil
		}
		e.Tombstoned = true
		e.LastSeenMS = g.now().UnixMilli()
		if err := g.store.PutEntity(e); err != nil {
			return false, err
		}
		g.publish(event.KindWorldForgotten, corr, map[string]any{"id": id, "name": e.Name, "what": "entity"})
		return true, nil
	}
	if r, found, err := g.store.GetRelation(id); err != nil {
		return false, err
	} else if found {
		if r.Tombstoned {
			return true, nil
		}
		r.Tombstoned = true
		r.LastSeenMS = g.now().UnixMilli()
		if err := g.store.PutRelation(r); err != nil {
			return false, err
		}
		g.publish(event.KindWorldForgotten, corr, map[string]any{"id": id, "verb": string(r.Verb), "what": "relation"})
		return true, nil
	}
	return false, nil
}

// Get returns a single entity by id (any state).
func (g *Graph) Get(id string) (Entity, bool, error) { return g.store.GetEntity(id) }

// Entities returns every active (non-tombstoned, non-superseded) entity,
// sorted deterministically. Used by `agt world list`.
func (g *Graph) Entities() ([]Entity, error) {
	all, err := g.store.AllEntities()
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, e := range all {
		if e.Active() {
			out = append(out, e)
		}
	}
	return out, nil
}

// Relations returns every active relation, sorted deterministically.
func (g *Graph) Relations() ([]Relation, error) {
	all, err := g.store.AllRelations()
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, r := range all {
		if r.Active() {
			out = append(out, r)
		}
	}
	return out, nil
}

// Count returns the number of stored entities (all states). Used by `agt status`.
func (g *Graph) Count() int { return g.store.Count() }

// publish writes one event through the bus, returning the persisted event (or
// nil when no bus is wired, e.g. store-only tests). Subject groups events
// under "worldmodel.<suffix>" so subscribers can scope-filter.
func (g *Graph) publish(kind event.Kind, corr string, payload any) *event.Event {
	if g.bus == nil {
		return nil
	}
	suffix := strings.TrimPrefix(string(kind), "worldmodel.")
	ev, _ := g.bus.Publish(event.Spec{
		Subject:       "worldmodel." + suffix,
		Kind:          kind,
		Actor:         "worldmodel",
		CorrelationID: corr,
		Payload:       payload,
	})
	return ev
}

func clampWeight(w float64) float64 {
	if w < 0 {
		return 0
	}
	if w > 1 {
		return 1
	}
	return w
}

func normalizeAliases(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, a := range in {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		key := strings.ToLower(a)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, a)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeAliases(existing, incoming []string) []string {
	return normalizeAliases(append(append([]string{}, existing...), incoming...))
}

// normalizeAttrs trims keys/values and drops empties, returning nil for an empty
// map so a cleared attr set round-trips as "no attrs" (matching the omitempty tag).
func normalizeAttrs(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeAttrs(existing, incoming map[string]string) map[string]string {
	if len(existing) == 0 {
		return incoming
	}
	out := make(map[string]string, len(existing)+len(incoming))
	maps.Copy(out, existing)
	maps.Copy(out, incoming)
	return out
}

// --- run-time context plumbing -------------------------------------------

type ctxKey int

const ctxKeyCorrelation ctxKey = iota

// WithCorrelation returns a child context carrying corr so the in-process