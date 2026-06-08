// SPDX-License-Identifier: MIT

package runstool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

type fakeJournal struct{ evs []*event.Event }

func (f *fakeJournal) Tail(n int) ([]*event.Event, error) { return f.evs, nil }

func ev(kind event.Kind, corr string, ts int64, payload any) *event.Event {
	b, _ := json.Marshal(payload)
	return &event.Event{Kind: kind, CorrelationID: corr, TSUnixMS: ts, Payload: b}
}

// sample journal: lead-1 completed ($0.002), lead-2 failed, lead-3 running,
// plus a sub-agent (child of lead-1) that must be folded OUT.
func sampleJournal() *fakeJournal {
	return &fakeJournal{evs: []*event.Event{
		ev(event.KindTaskReceived, "lead-1", 100, map[string]any{"intent": "research the Go website"}),
		ev(event.KindSubAgentSpawned, "lead-1", 101, map[string]any{"child_correlation": "sub-1"}),
		ev(event.KindTaskReceived, "sub-1", 102, map[string]any{"intent": "confirm the URL"}),
		ev(event.KindBudgetConsumed, "lead-1", 103, map[string]any{"cost_microcents": 2_000_000}),
		ev(event.KindTaskCompleted, "sub-1", 104, nil),
		ev(event.KindTaskCompleted, "lead-1", 105, nil),
		ev(event.KindTaskReceived, "lead-2", 200, map[string]any{"intent": "deploy the service"}),
		ev(event.KindTaskFailed, "lead-2", 201, map[string]any{"reason": "max_iters"}),
		ev(event.KindTaskReceived, "lead-3", 300, map[string]any{"intent": "ongoing analysis"}),
	}}
}

func newTool() *Tool {
	t := New()
	t.hist = sampleJournal()
	return t
}

func invoke(t *testing.T, tool *Tool, in map[string]any) (map[string]any, bool) {
	t.Helper()
	raw, _ := json.Marshal(in)
	res, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(res.Output), &out)
	return out, res.IsError
}

func TestDefinitionValid(t *testing.T) {
	d := New().Definition()
	if d.Name != "runs" || !json.Valid(d.InputSchema) {
		t.Fatalf("bad definition: %+v", d)
	}
}

func TestRecent_ExcludesSubAgents_NewestFirst(t *testing.T) {
	out, isErr := invoke(t, newTool(), map[string]any{"op": "recent"})
	if isErr {
		t.Fatalf("unexpected error: %v", out)
	}
	runs := out["runs"].([]any)
	if len(runs) != 3 {
		t.Fatalf("want 3 top-level runs (sub-agent folded out), got %d", len(runs))
	}
	// Newest first → lead-3 (ts 300), lead-2 (201), lead-1 (105).
	first := runs[0].(map[string]any)
	if first["id"] != "lead-3" {
		t.Errorf("newest run id = %v, want lead-3", first["id"])
	}
	// No sub-agent in the results.
	for _, r := range runs {
		if r.(map[string]any)["id"] == "sub-1" {
			t.Error("sub-agent run must be folded out")
		}
	}
}

func TestRecent_FoldsIntentStatusSpend(t *testing.T) {
	out, _ := invoke(t, newTool(), map[string]any{"op": "recent"})
	runs := out["runs"].([]any)
	byID := map[string]map[string]any{}
	for _, r := range runs {
		m := r.(map[string]any)
		byID[m["id"].(string)] = m
	}
	if byID["lead-1"]["status"] != "completed" || byID["lead-1"]["intent"] != "research the Go website" {
		t.Errorf("lead-1 folded wrong: %+v", byID["lead-1"])
	}
	if byID["lead-1"]["spent_microcents"].(float64) != 2_000_000 {
		t.Errorf("lead-1 spend = %v, want 2000000", byID["lead-1"]["spent_microcents"])
	}
	if byID["lead-2"]["status"] != "failed" || byID["lead-2"]["reason"] != "max_iters" {
		t.Errorf("lead-2 folded wrong: %+v", byID["lead-2"])
	}
	if byID["lead-3"]["status"] != "running" {
		t.Errorf("lead-3 status = %v, want running", byID["lead-3"]["status"])
	}
}

func TestStats(t *testing.T) {
	out, _ := invoke(t, newTool(), map[string]any{"op": "stats"})
	if out["total"].(float64) != 3 || out["completed"].(float64) != 1 || out["failed"].(float64) != 1 || out["running"].(float64) != 1 {
		t.Fatalf("stats wrong: %+v", out)
	}
	// success_rate = completed / (completed+failed) = 1/2.
	if out["success_rate"].(float64) != 0.5 {
		t.Errorf("success_rate = %v, want 0.5", out["success_rate"])
	}
	if out["total_spent_microcents"].(float64) != 2_000_000 {
		t.Errorf("spend = %v", out["total_spent_microcents"])
	}
}

func TestSearch(t *testing.T) {
	out, _ := invoke(t, newTool(), map[string]any{"op": "search", "query": "DEPLOY"})
	if out["matches"].(float64) != 1 {
		t.Fatalf("want 1 match for 'deploy', got %v", out["matches"])
	}
	runs := out["runs"].([]any)
	if runs[0].(map[string]any)["id"] != "lead-2" {
		t.Errorf("match id = %v, want lead-2", runs[0].(map[string]any)["id"])
	}
}

func TestSearch_EmptyQueryErrors(t *testing.T) {
	if _, isErr := invoke(t, newTool(), map[string]any{"op": "search"}); !isErr {
		t.Error("empty query should be an error")
	}
}

func TestBadOps(t *testing.T) {
	for _, op := range []string{"", "bogus"} {
		if _, isErr := invoke(t, newTool(), map[string]any{"op": op}); !isErr {
			t.Errorf("op %q should be an error", op)
		}
	}
}

func TestUnboundIsSafe(t *testing.T) {
	res, err := New().Invoke(context.Background(), json.RawMessage(`{"op":"stats"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("an unbound tool should return an error result")
	}
}
