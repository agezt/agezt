// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"sync"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// fakePulse is a stand-in PulseController so the control-plane tests don't
// need a live engine.
type fakePulse struct {
	mu     sync.Mutex
	paused bool
}

func (f *fakePulse) StatusMap() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return map[string]any{"running": !f.paused, "paused": f.paused, "beats": int64(3), "dial": "balanced"}
}
func (f *fakePulse) Pause()  { f.mu.Lock(); f.paused = true; f.mu.Unlock() }
func (f *fakePulse) Resume() { f.mu.Lock(); f.paused = false; f.mu.Unlock() }

func TestPulseStatusDisabledWhenNoEngine(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Call(context.Background(), controlplane.CmdPulseStatus, nil)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if enabled, _ := res["enabled"].(bool); enabled {
		t.Fatal("with no engine wired, status must report enabled=false")
	}
}

func TestPulsePauseDisabledErrors(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	if _, err := c.Call(context.Background(), controlplane.CmdPulsePause, nil); err == nil {
		t.Fatal("pause with no engine should error")
	}
}

func TestPulseStatusPauseResumeWithEngine(t *testing.T) {
	_, srv, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	fp := &fakePulse{}
	srv.SetPulse(fp)
	ctx := context.Background()

	res, err := c.Call(ctx, controlplane.CmdPulseStatus, nil)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if enabled, _ := res["enabled"].(bool); !enabled {
		t.Fatal("status should report enabled=true when an engine is wired")
	}
	if running, _ := res["running"].(bool); !running {
		t.Fatal("engine should start running")
	}

	if _, err := c.Call(ctx, controlplane.CmdPulsePause, nil); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if !fp.paused {
		t.Fatal("pause should have paused the engine")
	}

	if _, err := c.Call(ctx, controlplane.CmdPulseResume, nil); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if fp.paused {
		t.Fatal("resume should have resumed the engine")
	}
}
