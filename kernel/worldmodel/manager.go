// SPDX-License-Identifier: MIT

package worldmodel

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
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
// world tool can journal its writes under the originating run.
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

// --- agent tool -----------------------------------------------------------

const toolInputSchema = `{
  "type": "object",
  "properties": {
    "action":  {"type": "string", "enum": ["add", "relate", "resolve", "neighbors"], "description": "what to do"},
    "kind":    {"type": "string", "description": "entity kind for add (project|repo|person|org|account|device|channel|topic|task; default topic)"},
    "name":    {"type": "string", "description": "entity name (add)"},
    "aliases": {"type": "array", "items": {"type": "string"}, "description": "alternative phrases that resolve to this entity (add)"},
    "from":    {"type": "string", "description": "source entity name (relate)"},
    "verb":    {"type": "string", "description": "relation verb (relate; owns|depends_on|member_of|prefers|relates_to|assigned_to|derived_from)"},
    "to":      {"type": "string", "description": "target entity name (relate)"},
    "query":   {"type": "string", "description": "phrase to resolve to entities (resolve), or an entity name (neighbors)"},
    "limit":   {"type": "integer", "description": "max results (resolve; default 5)"}
  },
  "required": ["action"]
}`

type toolInput struct {
	Action  string            `json:"action"`
	Kind    Kind              `json:"kind"`
	Name    string            `json:"name"`
	Aliases []string          `json:"aliases"`
	Attrs   map[string]string `json:"attrs"`
	From    string            `json:"from"`
	Verb    Verb              `json:"verb"`
	To      string            `json:"to"`
	Query   string            `json:"query"`
	Limit   int               `json:"limit"`
}

type worldTool struct{ g *Graph }

// Tool returns the agent-facing world-model tool. Register it under the name
// "world" in the agent loop's tool map.
func (g *Graph) Tool() agent.Tool { return worldTool{g: g} }

func (t worldTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "world",
		Description: "Read and grow the world model — the graph of the operator's projects, repos, " +
			"people and topics and how they relate. action=add records an entity (kind, name, aliases); " +
			"action=relate links two entities (from, verb, to); action=resolve looks up what a phrase " +
			"refers to (query); action=neighbors lists what an entity connects to (query=name).",
		InputSchema: json.RawMessage(toolInputSchema),
	}
}

func (t worldTool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid world input: " + err.Error(), IsError: true}, nil
	}
	corr := CorrelationFrom(ctx)
	switch strings.ToLower(strings.TrimSpace(in.Action)) {
	case "add":
		e, created, err := t.g.Upsert(corr, UpsertSpec{Kind: in.Kind, Name: in.Name, Aliases: in.Aliases, Attrs: in.Attrs})
		if err != nil {
			return agent.Result{Output: "add failed: " + err.Error(), IsError: true}, nil
		}
		verb := "reinforced"
		if created {
			verb = "added"
		}
		return agent.Result{Output: fmt.Sprintf("%s entity %s (%s: %s)", verb, e.ID[:12], e.Kind, e.Name)}, nil
	case "relate":
		r, err := t.g.Relate(corr, in.From, in.Verb, in.To)
		if err != nil {
			return agent.Result{Output: "relate failed: " + err.Error(), IsError: true}, nil
		}
		return agent.Result{Output: fmt.Sprintf("related %s %s %s", in.From, r.Verb, in.To)}, nil
	case "resolve":
		limit := in.Limit
		if limit <= 0 {
			limit = 5
		}
		hits, err := t.g.Resolve(corr, in.Query, limit)
		if err != nil {
			return agent.Result{Output: "resolve failed: " + err.Error(), IsError: true}, nil
		}
		return agent.Result{Output: renderResolve(in.Query, hits)}, nil
	case "neighbors":
		hits, err := t.g.ResolveQuiet(in.Query, 1)
		if err != nil || len(hits) == 0 {
			return agent.Result{Output: "no entity matches " + in.Query, IsError: true}, nil
		}
		ns, err := t.g.Neighbors(hits[0].Entity.ID)
		if err != nil {
			return agent.Result{Output: "neighbors failed: " + err.Error(), IsError: true}, nil
		}
		return agent.Result{Output: renderNeighbors(hits[0].Entity, ns)}, nil
	default:
		return agent.Result{Output: "unknown action " + in.Action + " (add|relate|resolve|neighbors)", IsError: true}, nil
	}
}

func renderResolve(phrase string, hits []ScoredEntity) string {
	if len(hits) == 0 {
		return fmt.Sprintf("%q resolves to nothing known", phrase)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q resolves to:\n", phrase)
	for _, h := range hits {
		fmt.Fprintf(&b, "- [%s] %s\n", h.Entity.Kind, h.Entity.Name)
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderNeighbors(e Entity, ns []Neighbor) string {
	if len(ns) == 0 {
		return e.Name + " has no known relations"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s connects to:\n", e.Name)
	for _, n := range ns {
		other := n.Other.Name
		if other == "" {
			other = "(forgotten)"
		}
		if n.Outgoing {
			fmt.Fprintf(&b, "- %s %s\n", n.Relation.Verb, other)
		} else {
			fmt.Fprintf(&b, "- %s %s (incoming)\n", n.Relation.Verb, other)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
