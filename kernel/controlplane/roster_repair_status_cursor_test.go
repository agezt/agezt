// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"reflect"
	"strconv"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// Paging the repair history must not move the agent's current state: latest,
// next_eligible_ms and next_action describe the newest repair row regardless of
// the cursor. The legacy in-place cursor filter overwrote the full row list.
func TestAgentRepairStatus_CursorKeepsCurrentState(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("repair complete")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "paged", "model": "mock-model", "task_type": "code"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := k.Bus().Publish(event.Spec{
			Subject: "doctor.auto_repair",
			Kind:    event.KindInfo,
			Actor:   "kernel",
			Payload: map[string]any{"agent": "paged", "fingerprint": "fp-" + strconv.Itoa(i), "phase": "completed", "reason": "event " + strconv.Itoa(i)},
		}); err != nil {
			t.Fatalf("publish repair %d: %v", i, err)
		}
	}
	first, err := c.Call(ctx, controlplane.CmdAgentRepairStatus, map[string]any{"ref": "paged", "limit": 2})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	second, err := c.Call(ctx, controlplane.CmdAgentRepairStatus, map[string]any{"ref": "paged", "limit": 2, "cursor": first["next_cursor"]})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	latest, _ := first["latest"].(map[string]any)
	if latest["fingerprint"] != "fp-4" {
		t.Fatalf("page 1 latest = %v, want fp-4", latest["fingerprint"])
	}
	for _, key := range []string{"latest", "next_eligible_ms", "next_action"} {
		if !reflect.DeepEqual(first[key], second[key]) {
			t.Errorf("%s moved with the cursor:\npage 1 %v\npage 2 %v", key, first[key], second[key])
		}
	}
	history, _ := second["history"].([]any)
	if len(history) != 2 || history[0].(map[string]any)["fingerprint"] != "fp-2" {
		t.Fatalf("page 2 history = %v", history)
	}
}
