// SPDX-License-Identifier: MIT

// World model manager core: NewGraph + Upsert + EditEntity + Relate.
// Code extracted from manager.go during the Day-65 god-file split. Public API unchanged.
package worldmodel


import (
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"strings"
	"sync"
	"time"
)



// Graph wraps a Store with the kernel bus so every node/edge mutation is
// journaled (durable-before-publish) under the originating run's
// correlation_id. This is the journaling boundary — the Store stays pure. It
// mirrors kernel/memory.Manager exactly, for the same reason: `agt why` must
// be able to explain why the graph believes what it believes (SPEC-05 §3.3).
type Graph struct {
	store Store
	bus   *bus.Bus
	// now is the clock, injectable for deterministic tests. Defaults to
	// time.Now when constructed via NewGraph.
	now func() time.Time
	// mu serialises the read-modify-write mutators (Upsert/Relate/Forget/Decay) so a
	// reinforce can't race a Decay (or another reinforce) and lose an update — e.g.
	// Decay clobbering a just-refreshed weight (M421). The Store guards each call
	// individually but not the Get→Put pair.
	mu sync.Mutex
}

// NewGraph wires a Store to a bus. bus may be nil in tests that only exercise
// store-facing behaviour; production callers always pass the kernel bus so
// mutations are auditable.
func NewGraph(store Store, b *bus.Bus) *Graph {
	return &Graph{store: store, bus: b, now: time.Now}
}

// UpsertSpec is the input to Upsert.
type UpsertSpec struct {
	Kind    Kind
	Name    string
	Aliases []string
	Attrs   map[string]string
	Weight  float64
}

// Upsert creates (or reinforces) an entity and journals the write.
// Content-addressing by (kind, name) means re-adding the same entity dedupes
// onto the existing node: its recency is refreshed, its weight nudged up
// ("re-observed"), and any new aliases/attrs are merged in. A tombstoned node
// is revived. Returns the entity and whether it was newly created.
func (g *Graph) Upsert(corr string, spec UpsertSpec) (Entity, bool, error) {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return Entity{}, false, ErrEmptyName
	}
	kind := NormalizeKind(spec.Kind)
	w := spec.Weight
	if w <= 0 {
		w = 1.0
	}
	w = clampWeight(w)
	nowMS := g.now().UnixMilli()
	id := EntityID(kind, name)

	// Hold the lock across the Get→Put so a concurrent reinforce/decay can't lose it.
	g.mu.Lock()
	defer g.mu.Unlock()

	existing, found, err := g.store.GetEntity(id)
	if err != nil {
		return Entity{}, false, err
	}

	e := Entity{
		ID:         id,
		Kind:       kind,
		Name:       name,
		Aliases:    normalizeAliases(spec.Aliases),
		Attrs:      spec.Attrs,
		Weight:     w,
		CreatedMS:  nowMS,
		LastSeenMS: nowMS,
	}
	action := "create"
	if found {
		e.CreatedMS = existing.CreatedMS
		e.SourceEvent = existing.SourceEvent
		e.Weight = clampWeight(existing.Weight + 0.1)
		e.Aliases = mergeAliases(existing.Aliases, e.Aliases)
		e.Attrs = mergeAttrs(existing.Attrs, spec.Attrs)
		// Preserve a supersession link: reinforcing an entity that was explicitly
		// superseded must not resurrect it as active (it has a designated successor).
		e.SupersededBy = existing.SupersededBy
		action = "reinforce"
		if existing.Tombstoned {
			action = "revive"
		}
	}

	ev := g.publish(event.KindWorldEntityUpserted, corr, map[string]any{
		"action": action,
		"id":     id,
		"kind":   string(kind),
		"name":   name,
		"weight": e.Weight,
	})
	if ev != nil && e.SourceEvent == "" {
		e.SourceEvent = ev.ID
	}
	if err := g.store.PutEntity(e); err != nil {
		return Entity{}, false, err
	}
	return e, !found, nil
}

// EditEntity replaces an existing entity's aliases and attrs with the supplied
// values — the full editable state, so an alias or attr can be REMOVED (unlike
// Upsert, which only ever merges new values in). Identity (id/kind/name), weight,
// provenance and timestamps are preserved; LastSeenMS is refreshed. Journals
// world.entity.upserted with action "edit". Returns the updated entity and whether
// the id existed (false + nil error when unknown). A tombstoned/superseded entity
// keeps those flags — editing doesn't revive it (Upsert does that).
func (g *Graph) EditEntity(corr, id string, aliases []string, attrs map[string]string) (Entity, bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	e, found, err := g.store.GetEntity(id)
	if err != nil {
		return Entity{}, false, err
	}
	if !found {
		return Entity{}, false, nil
	}
	e.Aliases = normalizeAliases(aliases)
	e.Attrs = normalizeAttrs(attrs)
	e.LastSeenMS = g.now().UnixMilli()
	g.publish(event.KindWorldEntityUpserted, corr, map[string]any{
		"action": "edit", "id": e.ID, "kind": string(e.Kind), "name": e.Name, "weight": e.Weight,
	})
	if err := g.store.PutEntity(e); err != nil {
		return Entity{}, false, err
	}
	return e, true, nil
}

// Relate asserts a directed relation between two entities named fromName and
// toName. Each endpoint is resolved against existing entities by exact
// name/alias; an unknown endpoint is created as a topic entity so a relation
// never dangles. The edge is content-addressed (from, verb, to) and reinforced
// on re-assertion. Returns the relation.
func (g *Graph) Relate(corr, fromName string, verb Verb, toName string) (Relation, error) {
	from, err := g.resolveOrCreate(corr, fromName)
	if err != nil {
		return Relation{}, err
	}
	to, err := g.resolveOrCreate(corr, toName)
	if err != nil {
		return Relation{}, err
	}
	v := NormalizeVerb(verb)
	nowMS := g.now().UnixMilli()
	id := RelationID(from, v, to)

	// resolveOrCreate above may Upsert (self-locking); take the lock only now, for
	// the relation Get→Put. Distinct critical section, so no re-entrancy.
	g.mu.Lock()
	defer g.mu.Unlock()

	existing, found, err := g.store.GetRelation(id)
	if err != nil {
		return Relation{}, err
	}
	r := Relation{
		ID: id, From: from, Verb: v, To: to,
		Weight: 1.0, CreatedMS: nowMS, LastSeenMS: nowMS,
	}
	action := "create"
	if found {
		r.CreatedMS = existing.CreatedMS
		r.SourceEvent = existing.SourceEvent
		r.Weight = clampWeight(existing.Weight + 0.1)
		action = "reinforce"
		if existing.Tombstoned {
			action = "revive"
		}
	}
	ev := g.publish(event.KindWorldRelationUpserted, corr, map[string]any{
		"action": action, "id": id, "from": from, "verb": string(v), "to": to,
	})
	if ev != nil && r.SourceEvent == "" {
		r.SourceEvent = ev.ID
	}
	if err := g.store.PutRelation(r); err != nil {
		return Relation{}, err
	}
	return r, nil
}

// resolveOrCreate returns the id of the active entity that exactly matches name
// (by name or alias, any kind), creating a topic entity if none exists.