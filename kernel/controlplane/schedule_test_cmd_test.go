// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestScheduleTest_PreviewsFires(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	add, err := c.Call(ctx, controlplane.CmdScheduleAdd,
		map[string]any{"intent": "hourly job", "interval_sec": 3600})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	id, _ := add["id"].(string)

	res, err := c.Call(ctx, controlplane.CmdScheduleTest, map[string]any{"id": id, "count": 4})
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if found, _ := res["found"].(bool); !found {
		t.Fatal("schedule not found")
	}
	fires, _ := res["forecasts"].([]any)
	if len(fires) != 4 {
		t.Fatalf("got %d forecasts, want 4", len(fires))
	}
	// Hourly: consecutive fires should be ~3600s apart and strictly increasing.
	var prev int64
	for i, raw := range fires {
		m, _ := raw.(map[string]any)
		u := int64(m["unix"].(float64))
		if i > 0 && u-prev != 3600 {
			t.Errorf("fire %d gap = %d, want 3600", i, u-prev)
		}
		prev = u
	}

	// Unknown id → found:false.
	res, _ = c.Call(ctx, controlplane.CmdScheduleTest, map[string]any{"id": "nope"})
	if found, _ := res["found"].(bool); found {
		t.Error("unknown schedule should be found:false")
	}
}
