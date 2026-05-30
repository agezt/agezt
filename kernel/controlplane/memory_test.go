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

func TestMemoryGetRequiresID(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	if _, err := c.Call(context.Background(), controlplane.CmdMemoryGet, nil); err == nil {
		t.Error("get without id must error")
	}
}
