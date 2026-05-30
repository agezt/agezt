// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestReflectRunAndShow(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// show before any pass → not found.
	res, err := c.Call(ctx, controlplane.CmdReflectShow, nil)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if found, _ := res["found"].(bool); found {
		t.Error("no reflection yet → found should be false")
	}

	// run a pass → returns a report with a correlation.
	res, err = c.Call(ctx, controlplane.CmdReflectRun, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, ok := res["observations"].(map[string]any); !ok {
		t.Fatalf("run should return observations, got %v", res)
	}
	if corr, _ := res["correlation_id"].(string); corr == "" {
		t.Error("run should return a correlation_id")
	}

	// show now finds the latest report.
	res, err = c.Call(ctx, controlplane.CmdReflectShow, nil)
	if err != nil {
		t.Fatalf("show 2: %v", err)
	}
	if found, _ := res["found"].(bool); !found {
		t.Error("after a run, show should find the report")
	}
}
