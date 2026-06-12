// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestWatchdogNextDelay(t *testing.T) {
	p := watchdogPolicy{baseDelay: time.Second, maxDelay: 8 * time.Second}
	cases := []struct {
		consecutive int
		want        time.Duration
	}{
		{0, time.Second}, {1, time.Second}, {2, 2 * time.Second},
		{3, 4 * time.Second}, {4, 8 * time.Second}, {5, 8 * time.Second}, // capped
		{10, 8 * time.Second},
	}
	for _, c := range cases {
		if got := p.nextDelay(c.consecutive); got != c.want {
			t.Errorf("nextDelay(%d) = %s, want %s", c.consecutive, got, c.want)
		}
	}
}

// fakeProc exits immediately when wait() is called.
type fakeProc struct {
	mu     sync.Mutex
	killed bool
}

func (f *fakeProc) pid() int    { return 4242 }
func (f *fakeProc) wait() error { return nil }
func (f *fakeProc) kill() error {
	f.mu.Lock()
	f.killed = true
	f.mu.Unlock()
	return nil
}

func TestSuperviseLoop_RestartsThenStops(t *testing.T) {
	p := defaultWatchdogPolicy()
	ctx, cancel := context.WithCancel(context.Background())

	var spawns int
	spawn := func() (proc, error) {
		spawns++
		if spawns >= 3 {
			cancel() // after the 3rd start, ask the watchdog to stop
		}
		return &fakeProc{}, nil
	}
	// Fixed clock (uptime 0 → backoff grows, but we stop well before the crash cap).
	now := func() time.Time { return time.Unix(0, 0) }
	sleep := func(ctx context.Context, _ time.Duration) error { return ctx.Err() }

	err := superviseLoop(ctx, spawn, t.TempDir(), p, now, sleep, func(string, ...any) {})
	if err != nil {
		t.Fatalf("clean shutdown should return nil, got %v", err)
	}
	if spawns != 3 {
		t.Errorf("spawns = %d, want 3 (restarted twice then stopped)", spawns)
	}
}

func TestSuperviseLoop_CrashLoopGivesUp(t *testing.T) {
	p := watchdogPolicy{baseDelay: time.Millisecond, maxDelay: time.Millisecond, crashWindow: time.Hour, maxCrashes: 2}
	var spawns int
	spawn := func() (proc, error) {
		spawns++
		return &fakeProc{}, nil
	}
	now := func() time.Time { return time.Unix(0, 0) } // all crashes in the same instant → within window
	sleep := func(context.Context, time.Duration) error { return nil }

	err := superviseLoop(context.Background(), spawn, t.TempDir(), p, now, sleep, func(string, ...any) {})
	if err == nil {
		t.Fatal("a crash loop should return an error")
	}
	// maxCrashes=2 → two starts happen, then the 3rd attempt trips the guard
	// (refused before spawning), so the daemon was started exactly twice.
	if spawns != 2 {
		t.Errorf("spawns = %d, want 2 (gave up when the 3rd restart exceeded maxCrashes)", spawns)
	}
}

func TestSuperviseLoop_KillsChildOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fp := &fakeProc{}
	blocked := make(chan struct{})
	spawn := func() (proc, error) { return &blockingProc{fakeProc: fp, release: blocked}, nil }
	now := time.Now
	sleep := ctxSleep

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel() // ask to stop while the child is "running"
	}()
	err := superviseLoop(ctx, spawn, t.TempDir(), defaultWatchdogPolicy(), now, sleep, func(string, ...any) {})
	if err != nil {
		t.Fatalf("cancel should be a clean stop, got %v", err)
	}
	fp.mu.Lock()
	killed := fp.killed
	fp.mu.Unlock()
	if !killed {
		t.Error("the running child should have been killed on cancel")
	}
}

// blockingProc.wait blocks until kill() (mimics a long-running daemon).
type blockingProc struct {
	*fakeProc
	release chan struct{}
	once    sync.Once
}

func (b *blockingProc) wait() error {
	<-b.release
	return errors.New("killed")
}
func (b *blockingProc) kill() error {
	b.once.Do(func() { close(b.release) })
	return b.fakeProc.kill()
}
