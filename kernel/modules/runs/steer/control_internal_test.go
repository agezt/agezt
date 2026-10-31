// SPDX-License-Identifier: MIT

package steer

import (
	"context"
	"testing"
	"time"
)

// White-box tests for the Control's concurrency semantics (M608). These pin the
// pause/resume/step/inject behaviour deterministically without spinning up a
// full kernel run, so they can't flake on loop timing.

// waitReturned runs rc.Wait in a goroutine and reports on a channel when it
// returns. Used to assert "Wait is blocked" vs "Wait unblocked".
func waitReturned(rc *Control, ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		_ = rc.Wait(ctx)
		close(done)
	}()
	return done
}

func assertBlocked(t *testing.T, done <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-done:
		t.Fatalf("Wait returned but should have blocked: %s", msg)
	case <-time.After(40 * time.Millisecond):
	}
}

func assertUnblocked(t *testing.T, done <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("Wait stayed blocked but should have returned: %s", msg)
	}
}

func TestRunControl_NotPausedWaitReturnsImmediately(t *testing.T) {
	rc := NewControl()
	assertUnblocked(t, waitReturned(rc, context.Background()), "a non-paused run must not block in Wait")
}

func TestRunControl_PauseBlocksResumeReleases(t *testing.T) {
	rc := NewControl()
	rc.Pause(time.Time{})
	done := waitReturned(rc, context.Background())
	assertBlocked(t, done, "paused run")
	rc.Resume()
	assertUnblocked(t, done, "resumed run")
}

func TestRunControl_StepReleasesExactlyOne(t *testing.T) {
	rc := NewControl()
	rc.Pause(time.Time{})
	// First Wait blocks (paused).
	d1 := waitReturned(rc, context.Background())
	assertBlocked(t, d1, "paused before step")
	// step releases exactly one iteration...
	rc.Step()
	assertUnblocked(t, d1, "first Wait after step")
	// ...and re-pauses: a subsequent Wait blocks again.
	d2 := waitReturned(rc, context.Background())
	assertBlocked(t, d2, "second Wait after a single step must re-block")
	rc.Resume()
	assertUnblocked(t, d2, "resume after step")
}

func TestRunControl_WaitHonoursContext(t *testing.T) {
	rc := NewControl()
	rc.Pause(time.Time{})
	ctx, cancel := context.WithCancel(context.Background())
	done := waitReturned(rc, ctx)
	assertBlocked(t, done, "paused, ctx live")
	cancel()
	assertUnblocked(t, done, "ctx cancelled while paused must unblock Wait")
}

func TestRunControl_ResumeOnARunningRunIsANoOp(t *testing.T) {
	rc := NewControl()
	if rc.Resume() {
		t.Fatal("resuming a run that is not paused must be a no-op")
	}
}

func TestRunControl_InjectDrainOrderAndOnce(t *testing.T) {
	rc := NewControl()
	rc.Inject("first", false)
	rc.Inject("second", true)
	got := rc.Drain()
	if len(got) != 2 || got[0].Text != "first" || got[1].Text != "second" {
		t.Fatalf("Drain = %v want [first second] in order", got)
	}
	if got[0].Note || !got[1].Note {
		t.Errorf("Drain notes = [%v %v] want [false true]", got[0].Note, got[1].Note)
	}
	if again := rc.Drain(); again != nil {
		t.Errorf("second Drain = %v want nil (directives consumed once)", again)
	}
}

func TestRunControl_SnapshotReflectsState(t *testing.T) {
	rc := NewControl()
	if p, n, _ := rc.Snapshot(); p || n != 0 {
		t.Fatalf("fresh snapshot = (%v,%d) want (false,0)", p, n)
	}
	rc.Pause(time.Time{})
	rc.Inject("x", false)
	if p, n, _ := rc.Snapshot(); !p || n != 1 {
		t.Errorf("after pause+inject snapshot = (%v,%d) want (true,1)", p, n)
	}
}

func TestRunControl_PauseLeaseExpires(t *testing.T) {
	rc := NewControl()
	rc.Pause(time.Now().Add(20 * time.Millisecond))
	assertUnblocked(t, waitReturned(rc, context.Background()), "pause lease expiry should release Wait")
	if p, _, _ := rc.Snapshot(); p {
		t.Fatal("pause lease expired but run remains paused")
	}
}
