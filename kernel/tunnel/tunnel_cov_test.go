// SPDX-License-Identifier: MIT

package tunnel

import (
	"context"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newTestCmd builds a throwaway *exec.Cmd for exercising the process helpers
// directly. It is never started, so it never spawns a real process.
func newTestCmd() *exec.Cmd {
	return exec.Command("agezt-noop-does-not-run")
}

// TestExecRun_ScansOutput runs a real, short-lived subprocess and confirms
// execRun spawns it, scans its merged stdout/stderr, and delivers lines to the
// callback. `go version` is present in the test toolchain and exits quickly, so
// it is a safe, portable stand-in for a tunnel binary.
func TestExecRun_ScansOutput(t *testing.T) {
	var mu sync.Mutex
	var lines []string
	err := execRun(context.Background(), "go", []string{"version"}, func(l string) {
		mu.Lock()
		lines = append(lines, l)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("execRun(go version) returned error: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(lines) == 0 {
		t.Fatal("execRun captured no output lines from `go version`")
	}
}

// TestExecRun_StartError exercises the error path where the binary cannot be
// found: cmd.Start (or the pipe wiring) fails and execRun returns the error.
func TestExecRun_StartError(t *testing.T) {
	err := execRun(context.Background(), "agezt-tunnel-binary-does-not-exist-xyz", nil, func(string) {})
	if err == nil {
		t.Fatal("execRun with a missing binary should return an error")
	}
}

// TestExecRun_CancelKillsProcess drives the ctx-cancellation path so cmd.Cancel
// (→ killProcessTree) fires against a live child. A long-sleeping command stands
// in for a tunnel that never exits on its own.
func TestExecRun_CancelKillsProcess(t *testing.T) {
	name, args := sleepCommand(30)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = execRun(ctx, name, args, func(string) {})
		close(done)
	}()
	// Give the child time to actually start, then cancel to trigger the kill.
	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case <-done:
		// execRun returned promptly after the kill — good.
	case <-time.After(10 * time.Second):
		t.Fatal("execRun did not return after ctx cancellation (kill did not take effect)")
	}
}

// sleepCommand returns an OS-appropriate command that blocks for ~n seconds.
func sleepCommand(n int) (string, []string) {
	if runtime.GOOS == "windows" {
		// ping loopback n+1 times ≈ n seconds of blocking without extra deps.
		return "ping", []string{"-n", itoa(n + 1), "127.0.0.1"}
	}
	return "sleep", []string{itoa(n)}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// TestSetProcessGroup_NoPanic ensures setProcessGroup accepts a fresh command
// without panicking on any platform (it's a no-op on Windows, sets attrs on
// unix). Covers the helper directly.
func TestSetProcessGroup_NoPanic(t *testing.T) {
	_, _ = sleepCommand(1) // keep sleepCommand referenced even if execRun test is skipped
	// Build a command via the same path the supervisor uses; setProcessGroup is
	// invoked inside execRun, but call it here explicitly for direct coverage.
	// We use a throwaway *exec.Cmd created by CommandContext.
	cmd := newTestCmd()
	setProcessGroup(cmd) // must not panic
}

// TestKillProcessTree_NilSafe verifies killProcessTree tolerates a command that
// was never started (nil Process) — the guard branch.
func TestKillProcessTree_NilSafe(t *testing.T) {
	killProcessTree(nil)          // nil cmd
	killProcessTree(newTestCmd()) // non-nil cmd, nil Process
}

// TestStart_BackoffAndReset drives the supervisor loop with an injected runFunc
// and virtual clock so the backoff-doubling and healthy-uptime-reset branches
// run without real sleeping. The fake run returns immediately (simulating a
// tunnel that keeps dropping), then ctx is cancelled to stop the loop.
func TestStart_BackoffAndReset(t *testing.T) {
	// Virtual clock: nowFunc advances by a controllable delta per run; afterFunc
	// returns an already-fired channel so backoff waits are instant.
	origNow, origAfter := nowFunc, afterFunc
	defer func() { nowFunc, afterFunc = origNow, origAfter }()

	var virtual atomic.Int64 // nanoseconds
	nowFunc = func() time.Time {
		return time.Unix(0, virtual.Load())
	}
	afterFunc = func(d time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Unix(0, virtual.Load())
		return ch
	}

	var runs atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())

	tun := &Tunnel{
		cmd: []string{"faketunnel"},
		run: func(_ context.Context, _ string, _ []string, onLine func(string)) error {
			n := runs.Add(1)
			// First run is "unhealthy" (short): advance the clock only slightly,
			// which keeps backoff doubling. Emit a URL so setURL/onURL run.
			onLine("forwarding to https://x.trycloudflare.com now")
			if n == 3 {
				// Third run is "healthy": advance clock past healthyUptime so the
				// reset branch executes on the next iteration.
				virtual.Add(int64(healthyUptime + time.Second))
			}
			if n >= 5 {
				cancel() // stop the supervisor after enough iterations
			}
			return nil
		},
		onURL: func(string) {},
	}

	done := make(chan struct{})
	go func() { tun.Start(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Start did not return after ctx cancellation")
	}
	if runs.Load() < 5 {
		t.Fatalf("expected the supervisor to run the tunnel at least 5 times, got %d", runs.Load())
	}
}

// TestStart_ImmediateCancel covers the early-return guard: a ctx that's already
// cancelled must make Start return without running the tunnel at all.
func TestStart_ImmediateCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var ran atomic.Bool
	tun := &Tunnel{
		cmd: []string{"faketunnel"},
		run: func(context.Context, string, []string, func(string)) error {
			ran.Store(true)
			return nil
		},
	}
	tun.Start(ctx)
	if ran.Load() {
		t.Error("Start should not run the tunnel when ctx is already cancelled")
	}
}

// TestBuildCommand_UnknownProvider covers the default switch arm for an
// unrecognized provider string (not custom, not a known preset).
func TestBuildCommand_UnknownProvider(t *testing.T) {
	_, err := buildCommand(Config{Provider: "not-a-real-provider", TargetURL: "http://127.0.0.1:8787"})
	if err == nil {
		t.Fatal("buildCommand with an unknown provider should return an error")
	}
}

// TestBuildCommand_MissingTarget covers the "target URL required" error branch
// of every preset that needs a target (each preset has its own guard).
func TestBuildCommand_MissingTarget(t *testing.T) {
	for _, provider := range []string{"cloudflared", "ngrok", "tailscale", "tailscale-funnel"} {
		if _, err := buildCommand(Config{Provider: provider}); err == nil {
			t.Errorf("buildCommand(%q) with no TargetURL should error", provider)
		}
	}
}
