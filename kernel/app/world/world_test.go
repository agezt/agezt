// SPDX-License-Identifier: MIT

package world_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	appworld "github.com/agezt/agezt/kernel/app/world"
	graph "github.com/agezt/agezt/kernel/worldmodel"
)

func worldFixture(t *testing.T) (*graph.FileStore, *appworld.Service) {
	t.Helper()
	s, err := graph.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, appworld.New(graph.NewGraph(s, nil))
}

func TestGraphCurationRetainsIdentityReplacementAndReversibility(t *testing.T) {
	s, service := worldFixture(t)
	ctx := context.Background()
	added, err := service.Add(ctx, appworld.AddInput{Name: " Project ", Kind: " NEW-KIND ", Aliases: []string{"Alias"}, Attrs: map[string]string{"old": "value"}})
	if err != nil {
		t.Fatal(err)
	}
	if !added.Created || added.Name != "Project" || added.Kind != "new-kind" || added.ID == "" {
		t.Fatalf("add=%+v", added)
	}
	again, err := service.Add(ctx, appworld.AddInput{Name: "project", Kind: "new-kind"})
	if err != nil {
		t.Fatal(err)
	}
	if again.Created || again.ID != added.ID {
		t.Fatalf("reinforcement=%+v", again)
	}
	edited, err := service.Edit(ctx, appworld.EditInput{ID: added.ID, Aliases: []string{"Replacement"}, Attrs: map[string]string{"new": "value"}})
	if err != nil {
		t.Fatal(err)
	}
	if !edited.Updated || edited.ID == nil || *edited.ID != added.ID {
		t.Fatalf("edit=%+v", edited)
	}
	rec, _, err := s.GetEntity(added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Aliases) != 1 || rec.Aliases[0] != "Replacement" || len(rec.Attrs) != 1 || rec.Attrs["new"] != "value" {
		t.Fatalf("editable state was merged instead of replaced: %+v", rec)
	}
	forgotten, err := service.Forget(ctx, appworld.GetInput{ID: added.ID})
	if err != nil || !forgotten.Forgotten {
		t.Fatalf("forget=%+v err=%v", forgotten, err)
	}
	got, err := service.Get(ctx, appworld.GetInput{ID: added.ID})
	if err != nil || !got.Found || got.Entity == nil || !got.Entity.Tombstoned {
		t.Fatalf("retained get=%+v err=%v", got, err)
	}
	list, err := service.List(ctx, appworld.ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	if list.Count != 0 {
		t.Fatalf("forgotten entity still active=%+v", list)
	}
}

func TestGraphRelationshipsRetainDirectionFieldsAndQuietResolve(t *testing.T) {
	_, service := worldFixture(t)
	ctx := context.Background()
	a, err := service.Add(ctx, appworld.AddInput{Name: "Alpha", Aliases: []string{"the project"}})
	if err != nil {
		t.Fatal(err)
	}
	relation, err := service.Relate(ctx, appworld.RelateInput{From: "the project", To: "Beta", Verb: " DEPENDS_ON "})
	if err != nil {
		t.Fatal(err)
	}
	if relation.From != a.ID || relation.Verb != "depends_on" || relation.To == "" {
		t.Fatalf("relation=%+v", relation)
	}
	for _, tc := range []struct {
		query    string
		outgoing bool
		other    string
	}{{"Alpha", true, "Beta"}, {"Beta", false, "Alpha"}} {
		out, err := service.Neighbors(ctx, appworld.QueryInput{Query: tc.query})
		if err != nil {
			t.Fatal(err)
		}
		if !out.Found || out.Entity == nil || out.Entity.Name != tc.query || out.Count != 1 || out.Neighbors[0].Outgoing != tc.outgoing || out.Neighbors[0].Other.Name != tc.other || out.Neighbors[0].Verb != "depends_on" {
			t.Fatalf("neighbors %s=%+v", tc.query, out)
		}
	}
	resolved, err := service.Resolve(ctx, appworld.ResolveInput{Query: "the project", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Count != 1 || resolved.Results[0].Entity.ID != a.ID || resolved.Results[0].Score <= 0 {
		t.Fatalf("resolve=%+v", resolved)
	}
	list, err := service.List(ctx, appworld.ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	if list.Count != 2 || list.RelationCount != 1 || list.Edges[0].ID != relation.ID || list.Edges[0].Weight != 1 {
		t.Fatalf("graph list=%+v", list)
	}
}

func TestGraphWireRetainsEmptyAndLifecycleFields(t *testing.T) {
	s, service := worldFixture(t)
	ctx := context.Background()
	for _, tc := range []struct {
		value any
		want  string
	}{
		{func() any {
			out, err := service.List(ctx, appworld.ListInput{})
			if err != nil {
				t.Fatal(err)
			}
			return out
		}(), `{"entities":[],"count":0,"edges":[],"relation_count":0}`},
		{func() any {
			out, err := service.Get(ctx, appworld.GetInput{ID: "missing"})
			if err != nil {
				t.Fatal(err)
			}
			return out
		}(), `{"found":false}`},
		{func() any {
			out, err := service.Edit(ctx, appworld.EditInput{ID: "missing"})
			if err != nil {
				t.Fatal(err)
			}
			return out
		}(), `{"updated":false}`},
		{func() any {
			out, err := service.Neighbors(ctx, appworld.QueryInput{Query: "missing"})
			if err != nil {
				t.Fatal(err)
			}
			return out
		}(), `{"found":false,"neighbors":[],"count":0}`},
	} {
		raw, err := json.Marshal(tc.value)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != tc.want {
			t.Errorf("wire=%s want=%s", raw, tc.want)
		}
	}
	rec := graph.Entity{ID: "state", Kind: graph.KindTopic, Name: "State", Weight: 0, CreatedMS: 0, LastSeenMS: 0, Aliases: []string{"alias"}, Attrs: map[string]string{"attr": "value"}, SourceEvent: "source", SupersededBy: "new", Tombstoned: true}
	if err := s.PutEntity(rec); err != nil {
		t.Fatal(err)
	}
	out, err := service.Get(ctx, appworld.GetInput{ID: "state"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	entity := wire["entity"].(map[string]any)
	for key, value := range map[string]any{"name": "State", "weight": float64(0), "created_ms": float64(0), "last_seen_ms": float64(0), "source_event": "source", "superseded_by": "new", "tombstoned": true} {
		if entity[key] != value {
			t.Errorf("%s=%v want=%v", key, entity[key], value)
		}
	}
}

type failingGraphStore struct {
	graph.Store
	cause        error
	failEntities bool
	failGet      bool
}

func (s failingGraphStore) AllEntities() ([]graph.Entity, error) {
	if s.failEntities {
		return nil, s.cause
	}
	return s.Store.AllEntities()
}
func (s failingGraphStore) AllRelations() ([]graph.Relation, error) { return nil, s.cause }
func (s failingGraphStore) GetEntity(id string) (graph.Entity, bool, error) {
	if s.failGet {
		return graph.Entity{}, false, s.cause
	}
	return s.Store.GetEntity(id)
}
func TestGraphReadErrorsRetainStoreCauseAndOrder(t *testing.T) {
	s, _ := worldFixture(t)
	cause := errors.New("fixture graph unavailable")
	if err := s.PutEntity(graph.Entity{ID: "seed", Name: "Seed", Kind: graph.KindTopic, Weight: 1}); err != nil {
		t.Fatal(err)
	}
	for _, failEntities := range []bool{true, false} {
		service := appworld.New(graph.NewGraph(failingGraphStore{Store: s, cause: cause, failEntities: failEntities, failGet: true}, nil))
		_, err := service.List(context.Background(), appworld.ListInput{})
		if !errors.Is(err, cause) {
			t.Errorf("list cause=%v", err)
		}
		_, err = service.Neighbors(context.Background(), appworld.QueryInput{Query: "Seed"})
		if !errors.Is(err, cause) {
			t.Errorf("neighbors cause=%v", err)
		}
		_, err = service.Get(context.Background(), appworld.GetInput{ID: "seed"})
		if !errors.Is(err, cause) {
			t.Errorf("get cause=%v", err)
		}
		if failEntities {
			_, err = service.Resolve(context.Background(), appworld.ResolveInput{Query: "Seed", Limit: 10})
			if !errors.Is(err, cause) {
				t.Errorf("resolve cause=%v", err)
			}
		}
	}
}
