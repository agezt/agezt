// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestScheduleAddListRemove(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Add
	res, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{
		"intent": "summarise new commits", "interval_sec": 3600, "model": "sonnet",
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	id, _ := res["id"].(string)
	if id == "" {
		t.Fatal("add must return an id")
	}
	if sec, _ := res["interval_sec"].(float64); sec != 3600 {
		t.Errorf("interval_sec = %v", res["interval_sec"])
	}

	// List
	res, err = c.Call(ctx, controlplane.CmdScheduleList, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	list, _ := res["schedules"].([]any)
	if len(list) != 1 {
		t.Fatalf("list count = %d, want 1", len(list))
	}
	m, _ := list[0].(map[string]any)
	if m["intent"] != "summarise new commits" || m["source"] != "operator" || m["enabled"] != true {
		t.Errorf("listed entry = %v", m)
	}

	// Run now → marks it due
	res, err = c.Call(ctx, controlplane.CmdScheduleRun, map[string]any{"id": id})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if triggered, _ := res["triggered"].(bool); !triggered {
		t.Error("run should report triggered=true")
	}

	// Remove
	res, err = c.Call(ctx, controlplane.CmdScheduleRemove, map[string]any{"id": id})
	if err != nil {
		t.Fatalf("rm: %v", err)
	}
	if removed, _ := res["removed"].(bool); !removed {
		t.Error("rm should report removed=true")
	}

	// List is now empty
	res, _ = c.Call(ctx, controlplane.CmdScheduleList, nil)
	if list, _ := res["schedules"].([]any); len(list) != 0 {
		t.Errorf("after rm, list count = %d, want 0", len(list))
	}
}

func TestScheduleAddValidates(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{"interval_sec": 60}); err == nil {
		t.Error("add without intent must error")
	}
	if _, err := c.Call(ctx, controlplane.CmdScheduleAdd, map[string]any{"intent": "x", "interval_sec": 0}); err == nil {
		t.Error("add with interval_sec < 1 must error")
	}
}

func TestScheduleRemoveMissing(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Call(context.Background(), controlplane.CmdScheduleRemove, map[string]any{"id": "nope"})
	if err != nil {
		t.Fatalf("rm: %v", err)
	}
	if removed, _ := res["removed"].(bool); removed {
		t.Error("removing a missing id should report removed=false")
	}
}
