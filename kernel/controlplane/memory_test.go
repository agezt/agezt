// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestMemoryAddListGetCycle(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Add
	res, err := c.Call(ctx, controlplane.CmdMemoryAdd, map[string]any{
		"subject": "lictor",
		"content": "Agezt is a Go agentic OS",
		"type":    "FACT",
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if created, _ := res["created"].(bool); !created {
		t.Error("first add should report created=true")
	}
	id, _ := res["id"].(string)
	if id == "" {
		t.Fatal("add must return an id")
	}

	// List
	res, err = c.Call(ctx, controlplane.CmdMemoryList, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	recs, _ := res["records"].([]any)
	if len(recs) != 1 {
		t.Fatalf("list count = %d, want 1", len(recs))
	}

	// Get
	res, err = c.Call(ctx, controlplane.CmdMemoryGet, map[string]any{"id": id})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if found, _ := res["found"].(bool); !found {
		t.Fatal("get should find the record")
	}
}

// TestMemorySupersede revises a fact (M731): add → supersede with new content →
// the new record is active, the old one is gone from the active list (superseded),
// and the response reports superseded=true. A revise to identical content is a
// no-op (superseded=false). Empty content / missing old_id are rejected.
func TestMemorySupersede(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	add, err := c.Call(ctx, controlplane.CmdMemoryAdd, map[string]any{
		"subject": "owner-tz", "content": "Owner is in UTC", "type": "FACT",
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	oldID, _ := add["id"].(string)

	// Revise the content.
	sup, err := c.Call(ctx, controlplane.CmdMemorySupersede, map[string]any{
		"old_id": oldID, "subject": "owner-tz", "content": "Owner is in Istanbul, UTC+3", "type": "FACT",
	})
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}
	if ok, _ := sup["superseded"].(bool); !ok {
		t.Errorf("revise to new content should report superseded=true: %v", sup)
	}
	newID, _ := sup["new_id"].(string)
	if newID == "" || newID == oldID {
		t.Errorf("supersede should mint a new id, got %q (old %q)", newID, oldID)
	}

	// The active list now shows only the new record.
	list, _ := c.Call(ctx, controlplane.CmdMemoryList, nil)
	recs, _ := list["records"].([]any)
	if len(recs) != 1 {
		t.Fatalf("active count = %d, want 1 (old superseded)", len(recs))
	}
	r0, _ := recs[0].(map[string]any)
	if r0["content"] != "Owner is in Istanbul, UTC+3" {
		t.Errorf("active record is not the revision: %v", r0["content"])
	}

	// The old record still exists (soft update) and points forward.
	got, _ := c.Call(ctx, controlplane.CmdMemoryGet, map[string]any{"id": oldID})
	if found, _ := got["found"].(bool); !found {
		t.Error("old record should be retained, not deleted")
	}

	// A revise to identical content is a no-op.
	noop, err := c.Call(ctx, controlplane.CmdMemorySupersede, map[string]any{
		"old_id": newID, "subject": "owner-tz", "content": "Owner is in Istanbul, UTC+3", "type": "FACT",
	})
	if err != nil {
		t.Fatalf("noop supersede: %v", err)
	}
	if ok, _ := noop["superseded"].(bool); ok {
		t.Error("revise to identical content should report superseded=false")
	}

	// Validation.
	if _, err := c.Call(ctx, controlplane.CmdMemorySupersede, map[string]any{"content": "x"}); err == nil {
		t.Error("supersede without old_id must error")
	}
	if _, err := c.Call(ctx, controlplane.CmdMemorySupersede, map[string]any{"old_id": oldID}); err == nil {
		t.Error("supersede without content must error")
	}
}

func TestMemoryAddRejectsEmptyContent(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	if _, err := c.Call(context.Background(), controlplane.CmdMemoryAdd, map[string]any{"subject": "x"}); err == nil {
		t.Error("add without content must error")
	}
}

func TestMemorySearchRanks(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	_, _ = c.Call(ctx, controlplane.CmdMemoryAdd, map[string]any{"subject": "agezt", "content": "agezt kernel journals events"})
	_, _ = c.Call(ctx, controlplane.CmdMemoryAdd, map[string]any{"subject": "weather", "content": "it is sunny today"})

	res, err := c.Call(ctx, controlplane.CmdMemorySearch, map[string]any{"query": "agezt kernel"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	results, _ := res["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("search returned %d results, want 1 (weather should not match)", len(results))
	}
}

func TestMemorySearchRequiresQuery(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	if _, err := c.Call(context.Background(), controlplane.CmdMemorySearch, nil); err == nil {
		t.Error("search without query must error")
	}
}

func TestMemoryForgetExcludesFromList(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	res, _ := c.Call(ctx, controlplane.CmdMemoryAdd, map[string]any{"subject": "s", "content": "forget me"})
	id, _ := res["id"].(string)

	res, err := c.Call(ctx, controlplane.CmdMemoryForget, map[string]any{"id": id})
	if err != nil {
		t.Fatalf("forget: %v", err)
	}
	if ok, _ := res["forgotten"].(bool); !ok {
		t.Error("forget should report forgotten=true")
	}

	// Gone from list...
	res, _ = c.Call(ctx, controlplane.CmdMemoryList, nil)
	if recs, _ := res["records"].([]any); len(recs) != 0 {
		t.Fatalf("forgotten record must not appear in list, got %d", len(recs))
	}
	// ...but still gettable (reversibility) and marked tombstoned.
	res, _ = c.Call(ctx, controlplane.CmdMemoryGet, map[string]any{"id": id})
	if found, _ := res["found"].(bool); !found {
		t.Fatal("forgotten record must remain retrievable by id")
	}
	rec, _ := res["record"].(map[string]any)
	if tomb, _ := rec["tombstoned"].(bool); !tomb {
		t.Error("retrieved forgotten record should be marked tombstoned")
	}
}

// TestMemoryPromote (M915): a private (agent-scoped) record can be shared from
// the control plane — promote clears its scope tag; idempotent and safe on an
// unknown id (promoted=false).
func TestMemoryPromote(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	res, _ := c.Call(ctx, controlplane.CmdMemoryAdd, map[string]any{
		"subject": "ops-note", "content": "staging deploy needs the vault key",
		"tags": map[string]any{"scope": "ops"},
	})
	id, _ := res["id"].(string)

	res, err := c.Call(ctx, controlplane.CmdMemoryPromote, map[string]any{"id": id})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if ok, _ := res["promoted"].(bool); !ok {
		t.Errorf("promote should report promoted=true: %v", res)
	}
	res, _ = c.Call(ctx, controlplane.CmdMemoryGet, map[string]any{"id": id})
	rec, _ := res["record"].(map[string]any)
	if tags, _ := rec["tags"].(map[string]any); tags != nil && tags["scope"] != nil {
		t.Errorf("promoted record must have no scope tag: %v", tags)
	}

	// Unknown id → promoted=false, not an error.
	res, err = c.Call(ctx, controlplane.CmdMemoryPromote, map[string]any{"id": "no-such-id"})
	if err != nil {
		t.Fatalf("promote unknown: %v", err)
	}
	if ok, _ := res["promoted"].(bool); ok {
		t.Error("unknown id should report promoted=false")
	}
	// Missing id → error.
	if _, err := c.Call(ctx, controlplane.CmdMemoryPromote, nil); err == nil {
		t.Error("promote without id must error")
	}
}

func TestMemoryGetRequiresID(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	if _, err := c.Call(context.Background(), controlplane.CmdMemoryGet, nil); err == nil {
		t.Error("get without id must error")
	}
}
