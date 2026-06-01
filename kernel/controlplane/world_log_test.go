// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestWorldLog_ListsAndFilters — `agt world log` folds entity/relation upserts
// and forgets newest-first, with a --kind filter (M86).
func TestWorldLog_ListsAndFilters(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	k.Bus().Publish(event.Spec{
		Subject: "world", Kind: event.KindWorldEntityUpserted, Actor: "agent",
		Payload: map[string]any{"action": "observe", "id": "e1", "kind": "person", "name": "Ada"},
	})
	k.Bus().Publish(event.Spec{
		Subject: "world", Kind: event.KindWorldRelationUpserted, Actor: "agent",
		Payload: map[string]any{"action": "observe", "id": "r1", "from": "Ada", "verb": "wrote", "to": "Notes"},
	})
	k.Bus().Publish(event.Spec{
		Subject: "world", Kind: event.KindWorldForgotten, Actor: "agent",
		Payload: map[string]any{"id": "e1", "name": "Ada", "what": "entity"},
	})

	res, err := c.Call(context.Background(), controlplane.CmdWorldLog, nil)
	if err != nil {
		t.Fatal(err)
	}
	all, _ := res["ops"].([]any)
	if len(all) != 3 {
		t.Fatalf("ops = %d want 3", len(all))
	}
	// --kind relation → just the relation upsert.
	rres, err := c.Call(context.Background(), controlplane.CmdWorldLog,
		map[string]any{"kind": "relation"})
	if err != nil {
		t.Fatal(err)
	}
	rops, _ := rres["ops"].([]any)
	if len(rops) != 1 {
		t.Fatalf("--kind relation = %d want 1", len(rops))
	}
	m, _ := rops[0].(map[string]any)
	if m["what"] != "relation" || m["label"] != "Ada wrote Notes" {
		t.Errorf("relation op = %v / %v", m["what"], m["label"])
	}
}
