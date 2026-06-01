// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestMemoryLog_ListsAndFilters — `agt memory log` folds the memory lifecycle
// events newest-first, and --op scopes to one operation (M85).
func TestMemoryLog_ListsAndFilters(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	k.Bus().Publish(event.Spec{
		Subject: "memory", Kind: event.KindMemoryWritten, Actor: "agent",
		Payload: map[string]any{"action": "write", "id": "m1", "type": "FACT", "subject": "sky is blue"},
	})
	k.Bus().Publish(event.Spec{
		Subject: "memory", Kind: event.KindMemoryForgotten, Actor: "agent",
		Payload: map[string]any{"id": "m1", "subject": "sky is blue"},
	})
	k.Bus().Publish(event.Spec{
		Subject: "memory", Kind: event.KindMemorySuperseded, Actor: "agent",
		Payload: map[string]any{"old_id": "m2", "new_id": "m3"},
	})

	res, err := c.Call(context.Background(), controlplane.CmdMemoryLog, nil)
	if err != nil {
		t.Fatal(err)
	}
	all, _ := res["ops"].([]any)
	if len(all) != 3 {
		t.Fatalf("ops = %d want 3", len(all))
	}
	// Newest first: the superseded op.
	first, _ := all[0].(map[string]any)
	if first["op"] != "supersede" {
		t.Errorf("newest op = %v want supersede", first["op"])
	}

	// --op forgotten → just the forget.
	fres, err := c.Call(context.Background(), controlplane.CmdMemoryLog,
		map[string]any{"op": "forgotten"})
	if err != nil {
		t.Fatal(err)
	}
	fops, _ := fres["ops"].([]any)
	if len(fops) != 1 {
		t.Fatalf("--op forgotten = %d want 1", len(fops))
	}
	m, _ := fops[0].(map[string]any)
	if m["op"] != "forget" || m["id"] != "m1" {
		t.Errorf("forget op = %v / %v want forget / m1", m["op"], m["id"])
	}
}
