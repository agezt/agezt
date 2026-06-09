// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestWorldAddResolveNeighbors(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Add an entity with an alias.
	res, err := c.Call(ctx, controlplane.CmdWorldAdd, map[string]any{
		"name": "Lictor", "kind": "project", "aliases": []any{"the portfolio"},
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if created, _ := res["created"].(bool); !created {
		t.Error("first add should report created=true")
	}

	// Resolve the alias phrase back to the entity.
	res, err = c.Call(ctx, controlplane.CmdWorldResolve, map[string]any{"query": "the portfolio"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	results, _ := res["results"].([]any)
	if len(results) == 0 {
		t.Fatal("alias should resolve to the entity")
	}
	first, _ := results[0].(map[string]any)
	ent, _ := first["entity"].(map[string]any)
	if ent["name"] != "Lictor" {
		t.Errorf("resolved to %v, want Lictor", ent["name"])
	}

	// Relate + neighbors.
	if _, err := c.Call(ctx, controlplane.CmdWorldRelate, map[string]any{
		"from": "Lictor", "verb": "depends_on", "to": "go-stdlib",
	}); err != nil {
		t.Fatalf("relate: %v", err)
	}
	res, err = c.Call(ctx, controlplane.CmdWorldNeighbors, map[string]any{"query": "Lictor"})
	if err != nil {
		t.Fatalf("neighbors: %v", err)
	}
	if found, _ := res["found"].(bool); !found {
		t.Fatal("neighbors should find Lictor")
	}
	ns, _ := res["neighbors"].([]any)
	if len(ns) != 1 {
		t.Fatalf("expected 1 neighbor, got %d", len(ns))
	}
}

// TestWorldEdit edits an entity's aliases/attrs in place over the control plane
// (M730): add with attrs → edit (replace aliases, change/remove/add attrs) → get
// reflects the new state; an unknown id reports updated:false.
func TestWorldEdit(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	add, err := c.Call(ctx, controlplane.CmdWorldAdd, map[string]any{
		"name": "Ada", "kind": "person",
		"aliases": []any{"the boss"},
		"attrs":   map[string]any{"brief": "morning", "tz": "UTC"},
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	id, _ := add["id"].(string)
	if id == "" {
		t.Fatalf("add returned no id: %v", add)
	}

	edit, err := c.Call(ctx, controlplane.CmdWorldEdit, map[string]any{
		"id":      id,
		"aliases": []any{"ada k"},
		"attrs":   map[string]any{"brief": "evening, terse", "role": "owner"},
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if up, _ := edit["updated"].(bool); !up {
		t.Fatalf("edit should report updated=true: %v", edit)
	}

	// Get reflects the replaced state.
	got, err := c.Call(ctx, controlplane.CmdWorldGet, map[string]any{"id": id})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	ent, _ := got["entity"].(map[string]any)
	attrs, _ := ent["attrs"].(map[string]any)
	if attrs["brief"] != "evening, terse" || attrs["role"] != "owner" {
		t.Errorf("attrs not edited: %v", attrs)
	}
	if _, ok := attrs["tz"]; ok {
		t.Errorf("removed attr tz should be gone: %v", attrs)
	}
	aliases, _ := ent["aliases"].([]any)
	if len(aliases) != 1 || aliases[0] != "ada k" {
		t.Errorf("aliases not replaced: %v", aliases)
	}

	// Unknown id → updated:false, not an error.
	miss, err := c.Call(ctx, controlplane.CmdWorldEdit, map[string]any{"id": "deadbeef"})
	if err != nil {
		t.Fatalf("edit unknown id errored: %v", err)
	}
	if up, _ := miss["updated"].(bool); up {
		t.Error("editing an unknown id should report updated=false")
	}
}

func TestWorldAddRejectsEmptyName(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	if _, err := c.Call(context.Background(), controlplane.CmdWorldAdd, map[string]any{"kind": "project"}); err == nil {
		t.Error("add without name must error")
	}
}

func TestWorldListEmpty(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Call(context.Background(), controlplane.CmdWorldList, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	ents, ok := res["entities"].([]any)
	if !ok || len(ents) != 0 {
		t.Fatalf("empty world should return [], got %v", res["entities"])
	}
}

func TestWorldListReturnsEdges(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdWorldRelate, map[string]any{
		"from": "Lictor", "verb": "depends_on", "to": "go-stdlib",
	}); err != nil {
		t.Fatalf("relate: %v", err)
	}

	res, err := c.Call(ctx, controlplane.CmdWorldList, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if rc, _ := res["relation_count"].(float64); rc != 1 {
		t.Errorf("relation_count = %v, want 1", res["relation_count"])
	}
	edges, ok := res["edges"].([]any)
	if !ok || len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %v", res["edges"])
	}
	e, _ := edges[0].(map[string]any)
	if e["verb"] != "depends_on" {
		t.Errorf("edge verb = %v, want depends_on", e["verb"])
	}
	// from/to are entity ids; both must be non-empty so the graph can link nodes.
	if e["from"] == "" || e["to"] == "" {
		t.Errorf("edge endpoints must be set, got from=%v to=%v", e["from"], e["to"])
	}
}

func TestWorldForgetExcludesFromList(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	res, err := c.Call(ctx, controlplane.CmdWorldAdd, map[string]any{"name": "Ephemeral", "kind": "project"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	id, _ := res["id"].(string)
	if id == "" {
		t.Fatal("add returned no id")
	}

	res, err = c.Call(ctx, controlplane.CmdWorldForget, map[string]any{"id": id})
	if err != nil {
		t.Fatalf("forget: %v", err)
	}
	if ok, _ := res["forgotten"].(bool); !ok {
		t.Error("forget should report forgotten=true")
	}

	// Gone from the active list...
	res, _ = c.Call(ctx, controlplane.CmdWorldList, nil)
	if ents, _ := res["entities"].([]any); len(ents) != 0 {
		t.Fatalf("forgotten entity must not appear in list, got %d", len(ents))
	}
	// ...but still retrievable by id (reversibility) and marked tombstoned.
	res, _ = c.Call(ctx, controlplane.CmdWorldGet, map[string]any{"id": id})
	if found, _ := res["found"].(bool); !found {
		t.Fatal("forgotten entity must remain retrievable by id")
	}
	ent, _ := res["entity"].(map[string]any)
	if tomb, _ := ent["tombstoned"].(bool); !tomb {
		t.Error("retrieved forgotten entity should be marked tombstoned")
	}
}

func TestWorldForgetRequiresID(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	if _, err := c.Call(context.Background(), controlplane.CmdWorldForget, map[string]any{}); err == nil {
		t.Error("world_forget without id should error")
	}
}
