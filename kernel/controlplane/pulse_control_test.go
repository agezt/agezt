// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// fakePulse is a stand-in PulseController so the control-plane tests don't
// need a live engine.
type fakePulse struct {
	mu      sync.Mutex
	paused  bool
	beats   int
	cadence time.Duration
	dial    string
	flushed int
}

func (f *fakePulse) StatusMap() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return map[string]any{"running": !f.paused, "paused": f.paused, "beats": int64(3), "dial": "balanced"}
}
func (f *fakePulse) Pause()  { f.mu.Lock(); f.paused = true; f.mu.Unlock() }
func (f *fakePulse) Resume() { f.mu.Lock(); f.paused = false; f.mu.Unlock() }
func (f *fakePulse) Beat()   { f.mu.Lock(); f.beats++; f.mu.Unlock() }
func (f *fakePulse) SetCadence(d time.Duration) time.Duration {
	f.mu.Lock()
	f.cadence = d
	f.mu.Unlock()
	return d
}
func (f *fakePulse) SetDial(dial string) string {
	f.mu.Lock()
	f.dial = dial
	f.mu.Unlock()
	return dial
}
func (f *fakePulse) FlushDigest() int {
	f.mu.Lock()
	f.flushed++
	f.mu.Unlock()
	return 2 // pretend two items were held
}

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
	_, srv, c, baseDir := startPair(t, mock.New(mock.FinalText("ok")))
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

	// Beat (M756) reaches the engine.
	if _, err := c.Call(ctx, controlplane.CmdPulseBeat, nil); err != nil {
		t.Fatalf("beat: %v", err)
	}
	if fp.beats != 1 {
		t.Fatalf("beat should have triggered one beat, got %d", fp.beats)
	}

	// SetCadence (M757) reaches the engine with the seconds converted to a duration.
	res, err = c.Call(ctx, controlplane.CmdPulseCadence, map[string]any{"seconds": 45})
	if err != nil {
		t.Fatalf("cadence: %v", err)
	}
	if ms, _ := res["cadence_ms"].(float64); ms != 45000 {
		t.Fatalf("expected cadence_ms 45000, got %v", res["cadence_ms"])
	}
	if fp.cadence != 45*time.Second {
		t.Fatalf("expected engine cadence 45s, got %v", fp.cadence)
	}

	// SetDial (M758) reaches the engine and echoes the applied dial.
	res, err = c.Call(ctx, controlplane.CmdPulseDial, map[string]any{"dial": "chatty"})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if got, _ := res["dial"].(string); got != "chatty" {
		t.Fatalf("expected dial chatty, got %v", res["dial"])
	}
	if fp.dial != "chatty" {
		t.Fatalf("expected engine dial chatty, got %q", fp.dial)
	}

	// FlushDigest (M761) reaches the engine and returns the count flushed.
	res, err = c.Call(ctx, controlplane.CmdPulseFlush, nil)
	if err != nil {
		t.Fatalf("flush: %v", err)
	}
	if n, _ := res["flushed"].(float64); n != 2 {
		t.Fatalf("expected flushed 2, got %v", res["flushed"])
	}
	if fp.flushed != 1 {
		t.Fatalf("flush should have called the engine once, got %d", fp.flushed)
	}

	// Persistence (M760): cadence + dial are written to the config store so they
	// survive restart (buildPulse reads these env vars, overlaid from the store).
	store := settings.NewStore(baseDir)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	if v, ok := store.Get("AGEZT_PULSE_CADENCE"); !ok || v != (45 * time.Second).String() {
		t.Fatalf("cadence not persisted: %q (ok=%v)", v, ok)
	}
	if v, ok := store.Get("AGEZT_PULSE_DIAL"); !ok || v != "chatty" {
		t.Fatalf("dial not persisted: %q (ok=%v)", v, ok)
	}
}
