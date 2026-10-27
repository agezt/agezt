// SPDX-License-Identifier: MIT

package agentapi

import (
	"context"
	"testing"
)

// The wake provenance is a CONTRACT row: a host that starts a run states WHY
// the run exists, and it must be able to do that without naming the
// runtime's module. These tests hold the row's behaviour — it travels, it is
// trimmed, and an empty one is not attached at all.

func TestTheWakeContextTravels(t *testing.T) {
	w := WakeContext{Source: "schedule", Reason: "nightly", ScheduleID: "sch-1", TriggerSubject: "workboard.task-7"}
	ctx := WithWakeContext(context.Background(), w)
	got, ok := WakeContextFrom(ctx)
	if !ok {
		t.Fatal("the wake context did not travel")
	}
	if got != w {
		t.Errorf("wake context = %+v, want %+v", got, w)
	}
}

func TestTheWakeContextIsTrimmed(t *testing.T) {
	ctx := WithWakeContext(context.Background(), WakeContext{Source: "  operator  ", Reason: "  escalate  "})
	got, ok := WakeContextFrom(ctx)
	if !ok {
		t.Fatal("the wake context did not travel")
	}
	if got.Source != "operator" || got.Reason != "escalate" {
		t.Errorf("wake context = %+v, want the fields trimmed", got)
	}
}

func TestAnEmptyWakeContextIsNotAttached(t *testing.T) {
	ctx := WithWakeContext(context.Background(), WakeContext{})
	if _, ok := WakeContextFrom(ctx); ok {
		t.Error("an empty wake context must not be attached — the journal would carry a provenance that says nothing")
	}
}

func TestAPlainContextCarriesNoWake(t *testing.T) {
	if _, ok := WakeContextFrom(context.Background()); ok {
		t.Error("a plain context must carry no wake context")
	}
}
