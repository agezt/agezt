// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/memory"
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

// TestMemoryBulkForget soft-deletes multiple records in one call and reports
// forgotten vs not_found counts. It is idempotent: re-forgetting already-tombstoned
// records still counts as forgotten.
func TestMemoryBulkForget(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Add three records.
	r1, _ := c.Call(ctx, controlplane.CmdMemoryAdd, map[string]any{"subject": "a", "content": "content a"})
	r2, _ := c.Call(ctx, controlplane.CmdMemoryAdd, map[string]any{"subject": "b", "content": "content b"})
	r3, _ := c.Call(ctx, controlplane.CmdMemoryAdd, map[string]any{"subject": "c", "content": "content c"})
	id1, _ := r1["id"].(string)
	id2, _ := r2["id"].(string)
	id3, _ := r3["id"].(string)

	// Bulk-forget two of them plus a non-existent id.
	res, err := c.Call(ctx, controlplane.CmdMemoryBulkForget, map[string]any{
		"ids": []any{id1, id2, "does-not-exist"},
	})
	if err != nil {
		t.Fatalf("bulk_forget: %v", err)
	}
	if forgotten, _ := res["forgotten"].(float64); forgotten != 2 {
		t.Errorf("forgotten = %.0f, want 2", forgotten)
	}
	if notFound, _ := res["not_found"].(float64); notFound != 1 {
		t.Errorf("not_found = %.0f, want 1", notFound)
	}

	// Re-forgetting id1 should still count as forgotten (idempotent).
	res, _ = c.Call(ctx, controlplane.CmdMemoryBulkForget, map[string]any{
		"ids": []any{id1},
	})
	if f, _ := res["forgotten"].(float64); f != 1 {
		t.Errorf("re-forget of tombstoned id: forgotten = %.0f, want 1", f)
	}

	// Bulk-forget all remaining (id3) plus many non-existent.
	nonExistent := make([]any, 10)
	for i := range nonExistent {
		nonExistent[i] = "nonexistent-" + string(rune('0'+i))
	}
	ids := append(nonExistent, id3)
	res, _ = c.Call(ctx, controlplane.CmdMemoryBulkForget, map[string]any{"ids": ids})
	if f, _ := res["forgotten"].(float64); f != 1 {
		t.Errorf("remaining record: forgotten = %.0f, want 1", f)
	}
	if nf, _ := res["not_found"].(float64); nf != 10 {
		t.Errorf("remaining record: not_found = %.0f, want 10", nf)
	}
}

// TestMemoryBulkForgetValidation checks that missing or wrong-type ids are rejected.
func TestMemoryBulkForgetValidation(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Missing ids arg.
	if _, err := c.Call(context.Background(), controlplane.CmdMemoryBulkForget, nil); err == nil {
		t.Error("bulk_forget without ids must error")
	}

	// Wrong type for ids.
	if _, err := c.Call(context.Background(), controlplane.CmdMemoryBulkForget, map[string]any{"ids": "not-an-array"}); err == nil {
		t.Error("bulk_forget with string ids must error")
	}

	// Over 500 ids.
	tooMany := make([]any, 501)
	for i := range tooMany {
		tooMany[i] = "id-" + string(rune(i))
	}
	if _, err := c.Call(context.Background(), controlplane.CmdMemoryBulkForget, map[string]any{"ids": tooMany}); err == nil {
		t.Error("bulk_forget with >500 ids must error")
	}
}

// TestMemoryFindRelated uses hybrid search to find records related to a seed
// record's content. The seed itself is excluded from results.
func TestMemoryFindRelated(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Add a seed record about Go concurrency.
	seed, _ := c.Call(ctx, controlplane.CmdMemoryAdd, map[string]any{
		"subject": "concurrency", "content": "Goroutines and channels are Go's concurrency primitives",
	})
	seedID, _ := seed["id"].(string)

	// Add some related and unrelated records.
	c.Call(ctx, controlplane.CmdMemoryAdd, map[string]any{
		"subject": "goroutines", "content": "A goroutine is a lightweight thread managed by the Go runtime",
	})
	c.Call(ctx, controlplane.CmdMemoryAdd, map[string]any{
		"subject": "weather", "content": "It is raining outside today",
	})
	c.Call(ctx, controlplane.CmdMemoryAdd, map[string]any{
		"subject": "channels", "content": "Channels are the pipes that connect concurrent goroutines",
	})

	res, err := c.Call(ctx, controlplane.CmdMemoryFindRelated, map[string]any{
		"id":    seedID,
		"limit": 10,
	})
	if err != nil {
		t.Fatalf("find_related: %v", err)
	}
	results, _ := res["results"].([]any)
	count, _ := res["count"].(float64)

	// Should find the two Go-related records (goroutines + channels), not the seed itself.
	if count < 2 {
		t.Fatalf("find_related returned count=%.0f, want >=2 related records", count)
	}
	if int(count) != len(results) {
		t.Errorf("count = %d, len(results) = %d, should match", int(count), len(results))
	}
	for _, raw := range results {
		m, _ := raw.(map[string]any)
		rec, _ := m["record"].(map[string]any)
		if rec["id"] == seedID {
			t.Error("seed record should be excluded from results")
		}
	}

	// Result records should be about goroutines or channels.
	subjects := make(map[string]bool)
	for _, raw := range results {
		m, _ := raw.(map[string]any)
		rec, _ := m["record"].(map[string]any)
		subjects[rec["subject"].(string)] = true
	}
	if !subjects["goroutines"] || !subjects["channels"] {
		t.Errorf("expected goroutines + channels in results, got: %v", subjects)
	}
}

// TestMemoryFindRelatedRequiresID checks that missing id is rejected.
func TestMemoryFindRelatedRequiresID(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	if _, err := c.Call(context.Background(), controlplane.CmdMemoryFindRelated, nil); err == nil {
		t.Error("find_related without id must error")
	}
}

// TestMemoryFindRelatedSeedNotFound returns an error when the seed id does not exist.
func TestMemoryFindRelatedSeedNotFound(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	_, err := c.Call(context.Background(), controlplane.CmdMemoryFindRelated, map[string]any{"id": "does-not-exist"})
	if err == nil {
		t.Error("find_related with non-existent seed id must error")
	}
}

func TestMemoryAuditReportsContradictions(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	_, _ = c.Call(ctx, controlplane.CmdMemoryAdd, map[string]any{
		"subject": "region", "content": "prod is eu-west-1", "type": "FACT", "evidence": "curated",
	})
	_, _ = c.Call(ctx, controlplane.CmdMemoryAdd, map[string]any{
		"subject": "region", "content": "prod is us-east-1", "type": "FACT", "evidence": "curated",
	})

	res, err := c.Call(ctx, controlplane.CmdMemoryAudit, nil)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if load, _ := res["contradiction_load"].(float64); load != 1 {
		t.Fatalf("contradiction_load = %.0f, want 1: %v", load, res)
	}
	if usable, _ := res["usable"].(float64); usable != 2 {
		t.Fatalf("usable = %.0f, want 2: %v", usable, res)
	}
}

func TestMemoryCleanDryRunAndExecute(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	seed, _, err := k.Memory().Remember("seed", memory.RememberSpec{
		Subject: "test",
		Content: "ran go test ./... and it passed",
		Tags:    map[string]string{"source": "agent"},
		Actor:   "agent",
		Force:   true,
	})
	if err != nil {
		t.Fatalf("seed low-value memory: %v", err)
	}
	id := seed.ID

	res, err := c.Call(ctx, controlplane.CmdMemoryClean, map[string]any{"dry_run": true})
	if err != nil {
		t.Fatalf("clean dry-run: %v", err)
	}
	if rejected, _ := res["rejected"].(float64); rejected != 1 {
		t.Fatalf("dry-run rejected = %.0f, want 1: %v", rejected, res)
	}
	got, _ := c.Call(ctx, controlplane.CmdMemoryGet, map[string]any{"id": id})
	rec, _ := got["record"].(map[string]any)
	if tomb, _ := rec["tombstoned"].(bool); tomb {
		t.Fatal("dry-run must not tombstone")
	}

	res, err = c.Call(ctx, controlplane.CmdMemoryClean, map[string]any{"dry_run": false})
	if err != nil {
		t.Fatalf("clean execute: %v", err)
	}
	if removed, _ := res["removed"].(float64); removed != 1 {
		t.Fatalf("execute removed = %.0f, want 1: %v", removed, res)
	}
	got, _ = c.Call(ctx, controlplane.CmdMemoryGet, map[string]any{"id": id})
	if found, _ := got["found"].(bool); found {
		t.Fatal("execute should hard-delete low-value memory")
	}
}
