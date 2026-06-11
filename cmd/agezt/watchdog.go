// SPDX-License-Identifier: MIT

package main

// Self-healing watchdog (M840): `agezt watchdog` is the "keep it alive" service
// the owner asked for — a second, tiny supervisor process (the SAME binary, so
// agezt supervises itself) that spawns `agezt daemon`, waits for it, and respawns
// it whenever it exits, with exponential backoff and a crash-loop guard. Run it
// instead of `agezt daemon` (or install it as a service / scheduled task) and the
// daemon comes back on its own after a crash.

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/agezt/agezt/internal/brand"
)

// watchdogPolicy tunes the restart behaviour.
type watchdogPolicy struct {
	baseDelay   time.Duration // backoff for the first quick failure
	maxDelay    time.Duration // backoff ceiling
	resetUptime time.Duration // a daemon that ran at least this long resets the backoff
	crashWindow time.Duration // window over which crashes are counted
	maxCrashes  int           // more than this many restarts within crashWindow → give up
}

func defaultWatchdogPolicy() watchdogPolicy {
	return watchdogPolicy{
		baseDelay:   1 * time.Second,
		maxDelay:    30 * time.Second,
		resetUptime: 60 * time.Second,
		crashWindow: 2 * time.Minute,
		maxCrashes:  6,
	}
}

// nextDelay returns the backoff before the next start given the count of
// CONSECUTIVE quick failures (1-based): base, 2×base, 4×base, … capped at maxDelay.
func (p watchdogPolicy) nextDelay(consecutive int) time.Duration {
	if consecutive <= 1 {
		return p.baseDelay
	}
	d := p.baseDelay
	for i := 1; i < consecutive; i++ {
		d *= 2
		if d >= p.maxDelay {
			return p.maxDelay
		}
	}
	return d
}

// proc is the slice of a child process the supervisor needs — an interface so the
// loop is testable without spawning real processes.
type proc interface {
	pid() int
	wait() error
	kill() error
}

// execProc wraps a real *exec.Cmd.
type execProc struct{ cmd *exec.Cmd }

func (e *execProc) pid() int    { return e.cmd.Process.Pid }
func (e *execProc) wait() error { return e.cmd.Wait() }
func (e *execProc) kill() error { return e.cmd.Process.Kill() }

// pruneOlderThan drops timestamps before cutoff (keeping order).
func pruneOlderThan(ts []time.Time, cutoff time.Time) []time.Time {
	out := ts[:0]
	for _, t := range ts {
		if t.After(cutoff) {
			out = append(out, t)
		}
	}
	return out
}

// superviseLoop is the testable heart of the watchdog: spawn → wait → backoff →
// respawn, until ctx is cancelled (clean shutdown, returns nil) or a crash loop is
// detected (returns an error). spawn starts one daemon; now/sleep are injectable
// for tests; logf reports lifecycle. On ctx cancel mid-run it kills the child.
func superviseLoop(
	ctx context.Context,
	spawn func() (proc, error),
	p watchdogPolicy,
	now func() time.Time,
	sleep func(context.Context, time.Duration) error,
	logf func(string, ...any),
) error {
	var recent []time.Time
	consecutive := 0
	for {
		if ctx.Err() != nil {
			return nil
		}
		start := now()
		recent = append(recent, start)
		recent = pruneOlderThan(recent, start.Add(-p.crashWindow))
		if len(recent) > p.maxCrashes {
			return fmt.Errorf("watchdog: daemon restarted %d times within %s — looks like a crash loop, giving up", len(recent), p.crashWindow)
		}

		child, err := spawn()
		if err != nil {
			consecutive++
			delay := p.nextDelay(consecutive)
			logf("failed to start daemon: %v; retrying in %s", err, delay)
			if sleep(ctx, delay) != nil {
				return nil
			}
			continue
		}
		logf("daemon started (pid %d)", child.pid())

		// Wait for the child, but abandon the wait if we're asked to shut down.
		done := make(chan error, 1)
		go func() { done <- child.wait() }()
		select {
		case <-ctx.Done():
			_ = child.kill()
			<-done // reap
			logf("watchdog stopping; daemon killed")
			return nil
		case waitErr := <-done:
			uptime := now().Sub(start)
			if uptime >= p.resetUptime {
				consecutive = 0 // it ran fine for a while; forget the backoff
			}
			consecutive++
			delay := p.nextDelay(consecutive)
			logf("daemon exited after %s (%v); restarting in %s", uptime.Round(time.Second), waitErr, delay)
			if sleep(ctx, delay) != nil {
				return nil
			}
		}
	}
}

// ctxSleep sleeps for d or until ctx is cancelled (returning ctx.Err()).
func ctxSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// runWatchdog supervises `agezt daemon` using the same executable, restarting it
// on exit. SIGINT/SIGTERM stop the watchdog and the daemon cleanly.
func runWatchdog(stdout, stderr io.Writer) int {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "%s watchdog: cannot find own executable: %v\n", brand.Binary, err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logf := func(format string, a ...any) {
		fmt.Fprintf(stdout, "%s watchdog: %s\n", brand.Binary, fmt.Sprintf(format, a...))
	}
	logf("supervising %q daemon — it will be restarted if it exits (Ctrl-C to stop)", exe)

	spawn := func() (proc, error) {
		cmd := exec.Command(exe, "daemon")
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		cmd.Stdin = nil
		cmd.Env = os.Environ()
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return &execProc{cmd: cmd}, nil
	}

	if err := superviseLoop(ctx, spawn, defaultWatchdogPolicy(), time.Now, ctxSleep, logf); err != nil {
		fmt.Fprintf(stderr, "%s watchdog: %v\n", brand.Binary, err)
		return 1
	}
	return 0
}
