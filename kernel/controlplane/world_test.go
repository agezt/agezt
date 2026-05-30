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
