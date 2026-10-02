// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/update"
)

type fakeDrainer struct {
	halts, resumes int
	active         int // runs still active when a drain ends
}

func (f *fakeDrainer) DrainAndHalt(time.Duration) (bool, int) {
	f.halts++
	return f.active == 0, f.active
}
func (f *fakeDrainer) ResumeWith(string) { f.resumes++ }

// halted reports whether the kernel is left halted.
func (f *fakeDrainer) halted() bool { return f.halts > f.resumes }

func runApply(t *testing.T, k *fakeDrainer, apply updateApplyFunc) (bool, []string) {
	t.Helper()
	var kinds []string
	ok := applyAvailableUpdate(context.Background(), k, apply, &update.UpdateInfo{Version: "v9.9.9"},
		func(s event.Spec) { kinds = append(kinds, s.Subject) }, io.Discard, io.Discard)
	return ok, kinds
}

// A verification failure (download, checksum, signature) happens before
// Apply's drain step: the kernel must never be halted. The checker used to
// halt FIRST, leaving the daemon "running" but unable to run anything.
func TestApplyUpdate_VerificationFailureNeverHalts(t *testing.T) {
	k := &fakeDrainer{}
	ok, events := runApply(t, k, func(_ context.Context, _ *update.UpdateInfo, _ func(context.Context, time.Duration) update.DrainResult) error {
		return &update.ErrSignatureInvalid{Reason: "no release key configured"}
	})
	if ok {
		t.Fatal("a refused signature must not restart the daemon")
	}
	if k.halts != 0 {
		t.Fatalf("kernel halted %d time(s) before verification succeeded", k.halts)
	}
	if len(events) != 1 || events[0] != "update.failed" {
		t.Fatalf("events = %v, want [update.failed]", events)
	}
}

// A drain timeout aborts the update after the kernel was halted: it must be
// resumed, not left halted.
func TestApplyUpdate_DrainTimeoutResumesKernel(t *testing.T) {
	k := &fakeDrainer{active: 2}
	ok, _ := runApply(t, k, func(ctx context.Context, _ *update.UpdateInfo, drain func(context.Context, time.Duration) update.DrainResult) error {
		if res := drain(ctx, time.Second); res.Timeout {
			return update.ErrDrainTimeout
		}
		return nil
	})
	if ok {
		t.Fatal("a drain timeout must not restart the daemon")
	}
	if k.halted() {
		t.Fatalf("kernel left halted after an aborted update (halts=%d resumes=%d)", k.halts, k.resumes)
	}
}

// Any failure after the drain (e.g. the swap) also resumes.
func TestApplyUpdate_FailureAfterDrainResumesKernel(t *testing.T) {
	k := &fakeDrainer{}
	ok, _ := runApply(t, k, func(ctx context.Context, _ *update.UpdateInfo, drain func(context.Context, time.Duration) update.DrainResult) error {
		drain(ctx, time.Second)
		return errors.New("update: atomic rename failed")
	})
	if ok || k.halted() {
		t.Fatalf("ok=%v halted=%v, want no restart and a running kernel", ok, k.halted())
	}
}

// Success restarts with the kernel halted (drained), whether or not Apply
// ran a drain step (DrainTimeout 0 skips it).
func TestApplyUpdate_SuccessRestartsHalted(t *testing.T) {
	withDrain := &fakeDrainer{}
	ok, events := runApply(t, withDrain, func(ctx context.Context, _ *update.UpdateInfo, drain func(context.Context, time.Duration) update.DrainResult) error {
		drain(ctx, time.Second)
		return nil
	})
	if !ok || !withDrain.halted() || withDrain.halts != 1 {
		t.Fatalf("ok=%v halts=%d, want a restart after exactly one drain", ok, withDrain.halts)
	}
	if len(events) != 1 || events[0] != "update.applied" {
		t.Fatalf("events = %v, want [update.applied]", events)
	}

	noDrain := &fakeDrainer{}
	ok, _ = runApply(t, noDrain, func(context.Context, *update.UpdateInfo, func(context.Context, time.Duration) update.DrainResult) error {
		return nil
	})
	if !ok || !noDrain.halted() {
		t.Fatalf("ok=%v halted=%v, want new work stopped before the restart", ok, noDrain.halted())
	}
}
