// SPDX-License-Identifier: MIT

// Package world owns transport-independent world graph use cases. Native
// adapters retain admission while typed operation binding follows the move.
package world

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/opapi"
	graph "github.com/agezt/agezt/kernel/worldmodel"
)

type AddInput struct {
	Name    string            `json:"name"`
	Kind    string            `json:"kind,omitempty"`
	Aliases []string          `json:"aliases,omitempty"`
	Attrs   map[string]string `json:"attrs,omitempty"`
}
type EditInput struct {
	ID      string            `json:"id"`
	Aliases []string          `json:"aliases,omitempty"`
	Attrs   map[string]string `json:"attrs,omitempty"`
}
type RelateInput struct {
	From string `json:"from"`
	To   string `json:"to"`
	Verb string `json:"verb,omitempty"`
}
type ResolveInput struct {
	Query string `json:"query"`
	// Limit is admitted by the native adapter (default 10, maximum 100).
	Limit int `json:"limit,omitempty"`
}
type QueryInput struct {
	Query string `json:"query"`
}
type GetInput struct {
	ID string `json:"id"`
}
type ListInput struct{}

type Entity struct {
	ID           string            `json:"id"`
	Kind         string            `json:"kind"`
	Name         string            `json:"name"`
	Weight       float64           `json:"weight"`
	CreatedMS    int64             `json:"created_ms"`
	LastSeenMS   int64             `json:"last_seen_ms"`
	Aliases      []string          `json:"aliases,omitempty"`
	Attrs        map[string]string `json:"attrs,omitempty"`
	SourceEvent  string            `json:"source_event,omitempty"`
	SupersededBy string            `json:"superseded_by,omitempty"`
	Tombstoned   bool              `json:"tombstoned,omitempty"`
}
type Edge struct {
	ID     string  `json:"id"`
	From   string  `json:"from"`
	Verb   string  `json:"verb"`
	To     string  `json:"to"`
	Weight float64 `json:"weight"`
}
type AddOutput struct {
	ID      string `json:"id"`
	Created bool   `json:"created"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
}
type EditOutput struct {
	Updated bool    `json:"updated"`
	ID      *string `json:"id,omitempty"`
	Kind    *string `json:"kind,omitempty"`
	Name    *string `json:"name,omitempty"`
}
type RelateOutput struct {
	ID   string `json:"id"`
	From string `json:"from"`
	Verb string `json:"verb"`
	To   string `json:"to"`
}
type Hit struct {
	Entity Entity  `json:"entity"`
	Score  float64 `json:"score"`
}
type ResolveOutput struct {
	Results []Hit `json:"results"`
	Count   int   `json:"count"`
}
type Neighbor struct {
	Verb     string `json:"verb"`
	Outgoing bool   `json:"outgoing"`
	Other    Entity `json:"other"`
}
type NeighborsOutput struct {
	Found     bool       `json:"found"`
	Entity    *Entity    `json:"entity,omitempty"`
	Neighbors []Neighbor `json:"neighbors"`
	Count     int        `json:"count"`
}
type ListOutput struct {
	Entities      []Entity `json:"entities"`
	Count         int      `json:"count"`
	Edges         []Edge   `json:"edges"`
	RelationCount int      `json:"relation_count"`
}
type GetOutput struct {
	Found  bool    `json:"found"`
	Entity *Entity `json:"entity,omitempty"`
}
type ForgetOutput struct {
	Forgotten bool `json:"forgotten"`
}

type Service struct{ graph *graph.Graph }

func New(g *graph.Graph) *Service { return &Service{graph: g} }

func entityView(e graph.Entity) Entity {
	return Entity{ID: e.ID, Kind: string(e.Kind), Name: e.Name, Weight: e.Weight, CreatedMS: e.CreatedMS, LastSeenMS: e.LastSeenMS,
		Aliases: e.Aliases, Attrs: e.Attrs, SourceEvent: e.SourceEvent, SupersededBy: e.SupersededBy, Tombstoned: e.Tombstoned}
}

func (s *Service) Add(ctx context.Context, in AddInput) (AddOutput, error) {
	e, created, err := s.graph.Upsert(opapi.CorrelationFromContext(ctx), graph.UpsertSpec{Kind: graph.Kind(in.Kind), Name: in.Name, Aliases: in.Aliases, Attrs: in.Attrs})
	if err != nil {
		return AddOutput{}, err
	}
	return AddOutput{ID: e.ID, Created: created, Kind: string(e.Kind), Name: e.Name}, nil
}
func (s *Service) Edit(ctx context.Context, in EditInput) (EditOutput, error) {
	e, ok, err := s.graph.EditEntity(opapi.CorrelationFromContext(ctx), in.ID, in.Aliases, in.Attrs)
	if err != nil {
		return EditOutput{}, err
	}
	out := EditOutput{Updated: ok}
	if ok {
		kind := string(e.Kind)
		out.ID = &e.ID
		out.Kind = &kind
		out.Name = &e.Name
	}
	return out, nil
}
func (s *Service) Relate(ctx context.Context, in RelateInput) (RelateOutput, error) {
	r, err := s.graph.Relate(opapi.CorrelationFromContext(ctx), in.From, graph.Verb(in.Verb), in.To)
	if err != nil {
		return RelateOutput{}, err
	}
	return RelateOutput{ID: r.ID, From: r.From, Verb: string(r.Verb), To: r.To}, nil
}
func (s *Service) Resolve(_ context.Context, in ResolveInput) (ResolveOutput, error) {
	hits, err := s.graph.ResolveQuiet(in.Query, in.Limit)
	if err != nil {
		return ResolveOutput{}, err
	}
	out := make([]Hit, 0, len(hits))
	for _, h := range hits {
		out = append(out, Hit{Entity: entityView(h.Entity), Score: h.Score})
	}
	return ResolveOutput{Results: out, Count: len(out)}, nil
}
func (s *Service) Neighbors(_ context.Context, in QueryInput) (NeighborsOutput, error) {
	hits, err := s.graph.ResolveQuiet(in.Query, 1)
	if err != nil {
		return NeighborsOutput{}, err
	}
	if len(hits) == 0 {
		return NeighborsOutput{Neighbors: []Neighbor{}}, nil
	}
	center := hits[0].Entity
	ns, err := s.graph.Neighbors(center.ID)
	if err != nil {
		return NeighborsOutput{}, err
	}
	out := make([]Neighbor, 0, len(ns))
	for _, n := range ns {
		out = append(out, Neighbor{Verb: string(n.Relation.Verb), Outgoing: n.Outgoing, Other: entityView(n.Other)})
	}
	view := entityView(center)
	return NeighborsOutput{Found: true, Entity: &view, Neighbors: out, Count: len(out)}, nil
}
func (s *Service) List(_ context.Context, _ ListInput) (ListOutput, error) {
	ents, err := s.graph.Entities()
	if err != nil {
		return ListOutput{}, err
	}
	rels, err := s.graph.Relations()
	if err != nil {
		return ListOutput{}, err
	}
	out := make([]Entity, 0, len(ents))
	for _, e := range ents {
		out = append(out, entityView(e))
	}
	edges := make([]Edge, 0, len(rels))
	for _, r := range rels {
		edges = append(edges, Edge{ID: r.ID, From: r.From, Verb: string(r.Verb), To: r.To, Weight: r.Weight})
	}
	return ListOutput{Entities: out, Count: len(out), Edges: edges, RelationCount: len(rels)}, nil
}
func (s *Service) Get(_ context.Context, in GetInput) (GetOutput, error) {
	e, found, err := s.graph.Get(in.ID)
	if err != nil {
		return GetOutput{}, err
	}
	out := GetOutput{Found: found}
	if found {
		view := entityView(e)
		out.Entity = &view
	}
	return out, nil
}
func (s *Service) Forget(ctx context.Context, in GetInput) (ForgetOutput, error) {
	ok, err := s.graph.Forget(opapi.CorrelationFromContext(ctx), in.ID)
	if err != nil {
		return ForgetOutput{}, err
	}
	return ForgetOutput{Forgotten: ok}, nil
}
