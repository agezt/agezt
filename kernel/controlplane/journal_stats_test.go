// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestJournalStats_CountsAndBreaksDownByKind — journal stats folds the journal
// into a total event count, a per-kind breakdown, segment count and on-disk
// bytes, and the time span (M132).
func TestJournalStats_CountsAndBreaksDownByKind(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Publish a known mix: 2 task.received + 1 task.completed.
	pub := func(kind event.Kind) {
		t.Helper()
		if _, err := k.Bus().Publish(event.Spec{
			Subject: "task", Kind: kind, Actor: "a",
			CorrelationID: "r", Payload: map[string]string{"intent": "x"},
		}); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}
	pub(event.KindTaskReceived)
	pub(event.KindTaskReceived)
	pub(event.KindTaskCompleted)

	res, err := c.Call(context.Background(), controlplane.CmdJournalStats, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["events"]); got != 3 {
		t.Errorf("events = %d want 3", got)
	}
	byKind, ok := res["by_kind"].(map[string]any)
	if !ok {
		t.Fatalf("by_kind missing/wrong type: %T", res["by_kind"])
	}
	if got := intOf(byKind["task.received"]); got != 2 {
		t.Errorf("by_kind[task.received] = %d want 2", got)
	}
	if got := intOf(byKind["task.completed"]); got != 1 {
		t.Errorf("by_kind[task.completed] = %d want 1", got)
	}
	if got := intOf(res["segments"]); got < 1 {
		t.Errorf("segments = %d want >= 1", got)
	}
	if got := intOf(res["bytes"]); got <= 0 {
		t.Errorf("bytes = %d want > 0", got)
	}
	if got := intOf(res["newest_unix_ms"]); got <= 0 {
		t.Errorf("newest_unix_ms = %d want > 0", got)
	}
}
