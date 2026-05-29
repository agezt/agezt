// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// publishGrepFixture seeds the journal with a known mix of events
// so each test can assert exact filter behavior. Returns the
// kernel + helper to publish more.
func publishGrepFixture(t *testing.T) (*controlplane.Client, func()) {
	t.Helper()
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	publish := func(subject, kind, actor, corr string, payload string) {
		t.Helper()
		spec := event.Spec{
			Subject:       subject,
			Kind:          event.Kind(kind),
			Actor:         actor,
			CorrelationID: corr,
		}
		if payload != "" {
			spec.Payload = map[string]any{"raw": payload}
		}
		if _, err := k.Bus().Publish(spec); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}
	publish("task.received", "task.received", "operator", "run-A", "intent=A")
	publish("tool.invoked", "tool.invoked", "agent", "run-A", "shell rm -rf /tmp/x")
	publish("tool.result", "tool.result", "agent", "run-A", "ok")
	publish("task.completed", "task.completed", "agent", "run-A", "answer=done")
	publish("task.received", "task.received", "operator", "run-B", "intent=B")
	publish("task.completed", "task.completed", "agent", "run-B", "answer=skipped")
	return c, func() {}
}

// TestJournalGrep_KindFilter narrows to one Event.Kind.
func TestJournalGrep_KindFilter(t *testing.T) {
	c, cleanup := publishGrepFixture(t)
	defer cleanup()
	res, err := c.Call(context.Background(), controlplane.CmdJournalGrep, map[string]any{
		"kind": "task.completed",
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["count"]); got != 2 {
		t.Errorf("count = %d; want 2 (two task.completed events in fixture)", got)
	}
	events, _ := res["events"].([]any)
	for _, raw := range events {
		m, _ := raw.(map[string]any)
		if k, _ := m["kind"].(string); k != "task.completed" {
			t.Errorf("event with kind=%q leaked through kind filter", k)
		}
	}
}

// TestJournalGrep_CorrelationFilter scopes to one run.
func TestJournalGrep_CorrelationFilter(t *testing.T) {
	c, cleanup := publishGrepFixture(t)
	defer cleanup()
	res, err := c.Call(context.Background(), controlplane.CmdJournalGrep, map[string]any{
		"correlation_id": "run-A",
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	// Fixture has 4 events on run-A: received, tool.invoked,
	// tool.result, task.completed.
	if got := intOf(res["count"]); got != 4 {
		t.Errorf("count = %d; want 4 events on run-A", got)
	}
}

// TestJournalGrep_PatternHitsPayload — the substring search must
// reach into the event payload, not just the metadata fields.
func TestJournalGrep_PatternHitsPayload(t *testing.T) {
	c, cleanup := publishGrepFixture(t)
	defer cleanup()
	res, err := c.Call(context.Background(), controlplane.CmdJournalGrep, map[string]any{
		"pattern": "rm -rf",
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["count"]); got != 1 {
		t.Errorf("count = %d; want 1 (single tool.invoked payload contains the pattern)", got)
	}
}

// TestJournalGrep_PatternIsCaseInsensitive proves the substring
// match is case-folded so operators don't have to remember the
// exact casing the daemon emitted.
func TestJournalGrep_PatternIsCaseInsensitive(t *testing.T) {
	c, cleanup := publishGrepFixture(t)
	defer cleanup()
	lower, err := c.Call(context.Background(), controlplane.CmdJournalGrep, map[string]any{"pattern": "task.completed"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	upper, err := c.Call(context.Background(), controlplane.CmdJournalGrep, map[string]any{"pattern": "TASK.COMPLETED"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if intOf(lower["count"]) != intOf(upper["count"]) {
		t.Errorf("case-folded counts disagree: lower=%d upper=%d", intOf(lower["count"]), intOf(upper["count"]))
	}
}

// TestJournalGrep_FiltersAndTogether — kind=tool.invoked AND
// correlation=run-A should match exactly one event.
func TestJournalGrep_FiltersAndTogether(t *testing.T) {
	c, cleanup := publishGrepFixture(t)
	defer cleanup()
	res, err := c.Call(context.Background(), controlplane.CmdJournalGrep, map[string]any{
		"kind":           "tool.invoked",
		"correlation_id": "run-A",
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["count"]); got != 1 {
		t.Errorf("count = %d; want 1 (AND of kind+correlation)", got)
	}
}

// TestJournalGrep_LimitClampsResults — limit caps the result size
// AND short-circuits the walk early.
func TestJournalGrep_LimitClampsResults(t *testing.T) {
	c, cleanup := publishGrepFixture(t)
	defer cleanup()
	res, err := c.Call(context.Background(), controlplane.CmdJournalGrep, map[string]any{
		"pattern": "task",
		"limit":   float64(2),
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["count"]); got != 2 {
		t.Errorf("count = %d; want exactly 2 (limit-clamped)", got)
	}
}

// TestJournalGrep_HeadIsReported — clients need head for "we
// looked at seq=0..head" rendering, matching CmdJournalTail.
func TestJournalGrep_HeadIsReported(t *testing.T) {
	c, cleanup := publishGrepFixture(t)
	defer cleanup()
	res, err := c.Call(context.Background(), controlplane.CmdJournalGrep, map[string]any{
		"pattern": "task",
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	// 6 events published in publishGrepFixture; head should be 5
	// (0-based seq, last published).
	if got := intOf(res["head"]); got != 5 {
		t.Errorf("head = %d; want 5 (six 0-based events)", got)
	}
}
