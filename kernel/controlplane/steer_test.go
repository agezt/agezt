// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The live-steering commands route to the kernel and report ok=false for a run
// that isn't live (the timing-robust control-plane shape test — the kernel-side
// pause/inject/resume behaviour is covered in kernel/runtime). (M608)
func TestSteerCommands_UnknownRunReportsNotOk(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	for _, cmd := range []string{controlplane.CmdRunPause, controlplane.CmdRunResume, controlplane.CmdRunStep} {
		res, err := c.Call(context.Background(), cmd, map[string]any{"correlation": "no-such-run"})
		if err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
		if ok, _ := res["ok"].(bool); ok {
			t.Errorf("%s on an unknown run: ok=true want false", cmd)
		}
		if res["correlation"] != "no-such-run" {
			t.Errorf("%s did not echo correlation: %v", cmd, res["correlation"])
		}
	}

	res, err := c.Call(context.Background(), controlplane.CmdRunSteer,
		map[string]any{"correlation": "no-such-run", "directive": "focus"})
	if err != nil {
		t.Fatalf("run_steer: %v", err)
	}
	if acc, _ := res["accepted"].(bool); acc {
		t.Error("run_steer on an unknown run: accepted=true want false")
	}
}

// Missing required args are clear errors, not silent no-ops.
func TestSteerCommands_RejectMissingArgs(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	if _, err := c.Call(context.Background(), controlplane.CmdRunPause, nil); err == nil {
		t.Error("run_pause without correlation should error")
	}
	if _, err := c.Call(context.Background(), controlplane.CmdRunSteer,
		map[string]any{"correlation": "r1"}); err == nil {
		t.Error("run_steer without directive should error")
	}
}
