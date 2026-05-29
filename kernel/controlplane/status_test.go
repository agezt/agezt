// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestStatus_ReturnsExpectedShape asserts the wire fields the
// `agt status` CLI relies on. Counts come from the startPair rig:
// one tool ("shell"), zero active runs, freshly-opened kernel
// (uptime < 5s, journal empty).
func TestStatus_ReturnsExpectedShape(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdStatus, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	if got, _ := res["daemon"].(string); got != brand.Version {
		t.Errorf("daemon = %q want %q", got, brand.Version)
	}
	if got := intOf(res["protocol"]); got != brand.ProtocolVersion {
		t.Errorf("protocol = %d want %d", got, brand.ProtocolVersion)
	}
	if got := intOf(res["tools"]); got != 1 {
		t.Errorf("tools = %d want 1", got)
	}
	if got := intOf(res["active_runs"]); got != 0 {
		t.Errorf("active_runs = %d want 0", got)
	}
	if halted, _ := res["halted"].(bool); halted {
		t.Errorf("halted = true; want false on a freshly-started kernel")
	}
	if got := intOf(res["uptime_seconds"]); got < 0 || got > 5 {
		t.Errorf("uptime_seconds = %d; want 0..5 for a freshly-started kernel", got)
	}
	if got := intOf(res["journal_head"]); got != 0 {
		t.Errorf("journal_head = %d; want 0 on an empty journal", got)
	}
}

// TestStatus_ReflectsHaltAndJournalGrowth verifies the dynamic
// fields (halted, journal_head) actually track state. After
// halting and publishing one event, status must show halted=true
// and journal_head=1.
func TestStatus_ReflectsHaltAndJournalGrowth(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Halt via direct kernel access — exercises the same code path
	// CmdHalt would, without the overhead of a second round-trip.
	k.Halt()

	// Publish one event so journal_head moves off zero. This is the
	// same path real bus publishers take, so it journals normally
	// (not ephemeral).
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "test.status",
		Kind:    event.Kind("test.event"),
		Actor:   "test",
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	res, err := c.Call(context.Background(), controlplane.CmdStatus, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if halted, _ := res["halted"].(bool); !halted {
		t.Error("halted = false; want true after Halt()")
	}
	if got := intOf(res["journal_head"]); got != 1 {
		t.Errorf("journal_head = %d; want 1 after one publish", got)
	}
}
