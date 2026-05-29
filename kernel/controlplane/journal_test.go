// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestJournalTail_EmptyJournalReturnsZero covers the fresh-install
// case: no events have ever been written, head=0, tail returns an
// empty array (not null) and count=0. Operators piping into jq
// should never see a "cannot iterate over null" error.
func TestJournalTail_EmptyJournalReturnsZero(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdJournalTail, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["count"]); got != 0 {
		t.Errorf("count = %d want 0", got)
	}
	if got := intOf(res["head"]); got != -1 && got != 0 {
		// Journal.Head returns -1 on empty; handler may or may not
		// clamp. Accept either, but not anything else.
		t.Errorf("head = %d want 0 or -1", got)
	}
	events, ok := res["events"].([]any)
	if !ok {
		t.Fatalf("events wrong type: %T (want []any)", res["events"])
	}
	if len(events) != 0 {
		t.Errorf("events should be empty, got %d", len(events))
	}
}

// TestJournalTail_ReturnsLastNInOrder writes 10 events, requests
// the last 4. The response must be events seq=7..10 (oldest→newest),
// not seq=1..4 (the first four).
func TestJournalTail_ReturnsLastNInOrder(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	for i := 0; i < 10; i++ {
		if _, err := k.Bus().Publish(event.Spec{
			Subject: "test.tail",
			Kind:    event.Kind("test.event"),
			Actor:   "test",
		}); err != nil {
			t.Fatalf("Publish %d: %v", i, err)
		}
	}

	res, err := c.Call(context.Background(), controlplane.CmdJournalTail, map[string]any{"n": 4})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["count"]); got != 4 {
		t.Fatalf("count = %d want 4", got)
	}
	// Journal seqs are 0-based: 10 events → seqs 0..9, head=9.
	if got := intOf(res["head"]); got != 9 {
		t.Errorf("head = %d want 9", got)
	}
	events, _ := res["events"].([]any)
	if len(events) != 4 {
		t.Fatalf("events len = %d want 4", len(events))
	}
	// Last 4 of seqs 0..9 = 6, 7, 8, 9 in order.
	for i, raw := range events {
		m, _ := raw.(map[string]any)
		wantSeq := int64(6 + i)
		if got := intOf(m["seq"]); int64(got) != wantSeq {
			t.Errorf("events[%d].seq = %d want %d", i, got, wantSeq)
		}
	}
}

// TestJournalTail_NLargerThanHeadReturnsAll covers the "operator
// asked for last 100 but only 3 events exist" case. Must return
// all 3 (count=3), not nil, not error.
func TestJournalTail_NLargerThanHeadReturnsAll(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	for i := 0; i < 3; i++ {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "test.small",
			Kind:    event.Kind("test.event"),
			Actor:   "test",
		})
	}
	res, err := c.Call(context.Background(), controlplane.CmdJournalTail, map[string]any{"n": 100})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["count"]); got != 3 {
		t.Errorf("count = %d want 3", got)
	}
}
