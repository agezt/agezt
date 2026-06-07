// SPDX-License-Identifier: MIT

package pulse

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/state"
)

// --- test doubles ---------------------------------------------------------

type fakeObserver struct {
	name   string
	deltas []Delta
	err    error
}

func (f *fakeObserver) Name() string { return f.name }
func (f *fakeObserver) Poll(context.Context) ([]Delta, error) {
	return f.deltas, f.err
}

type capturingSink struct {
	mu     sync.Mutex
	briefs []Brief
}

func (c *capturingSink) Deliver(b Brief) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.briefs = append(c.briefs, b)
	return nil
}
func (c *capturingSink) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.briefs)
}

func newEngine(t *testing.T, cfg Config) (*Engine, *journal.Journal) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal: %v", err)
	}
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	cfg.Bus = bus.New(j)
	cfg.State = st
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }
	}
	t.Cleanup(func() { cfg.Bus.Close(); j.Close(); st.Close() })
	return New(cfg), j
}

func countKind(t *testing.T, j *journal.Journal, k event.Kind) int {
	t.Helper()
	n := 0
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == k {
			n++
		}
		return nil
	})
	return n
}

// --- tests ----------------------------------------------------------------

func TestTickEmitsFullChain(t *testing.T) {
	sink := &capturingSink{}
	obs := &fakeObserver{name: "fake", deltas: []Delta{{
		Source: "probe:ci", Kind: "probe_failed", Summary: "ci failed",
		Hints: map[string]string{"severity": "high"},
	}}}
	e, j := newEngine(t, Config{Observers: []Observer{obs}, Dial: DialBalanced, Sink: sink})

	e.tickOnce(context.Background())

	for _, k := range []event.Kind{
		event.KindPulseTick, event.KindObserverDelta, event.KindSalienceScored,
		event.KindInitiativeTaken, event.KindBriefingSent,
	} {
		if countKind(t, j, k) != 1 {
			t.Errorf("expected exactly one %s event", k)
		}
	}
	if sink.count() != 1 {
		t.Fatalf("alert should deliver one brief now, got %d", sink.count())
	}
}

func TestChainSharesCorrelation(t *testing.T) {
	obs := &fakeObserver{name: "f", deltas: []Delta{{Source: "s", Kind: "k", Summary: "x", Hints: map[string]string{"severity": "high"}}}}
	e, j := newEngine(t, Config{Observers: []Observer{obs}})
	e.tickOnce(context.Background())

	// The delta→score→initiative→brief events must share one correlation so
	// `agt why <brief>` walks the whole chain.
	corrs := map[event.Kind]string{}
	_ = j.Range(func(ev *event.Event) error {
		switch ev.Kind {
		case event.KindObserverDelta, event.KindSalienceScored, event.KindInitiativeTaken, event.KindBriefingSent:
			corrs[ev.Kind] = ev.CorrelationID
		}
		return nil
	})
	base := corrs[event.KindObserverDelta]
	if base == "" {
		t.Fatal("observer.delta missing correlation")
	}
	for k, c := range corrs {
		if c != base {
			t.Errorf("%s correlation %q != %q", k, c, base)
		}
	}
}

func TestDialQuietSuppressesNotify(t *testing.T) {
	sink := &capturingSink{}
	// medium severity → notify; quiet dial routes notify to digest (not now).
	obs := &fakeObserver{name: "f", deltas: []Delta{{Source: "s", Kind: "k", Summary: "meh", Hints: map[string]string{"severity": "medium"}}}}
	e, j := newEngine(t, Config{Observers: []Observer{obs}, Dial: DialQuiet, Sink: sink})
	e.tickOnce(context.Background())

	if sink.count() != 0 {
		t.Fatal("quiet dial must not send a notify-level brief immediately")
	}
	if countKind(t, j, event.KindBriefingSent) != 0 {
		t.Fatal("no briefing.sent until the digest flushes")
	}
	if countKind(t, j, event.KindInitiativeTaken) != 1 {
		t.Fatal("a digested item still records initiative.taken")
	}
}

func TestNoveltySuppressesRepeat(t *testing.T) {
	sink := &capturingSink{}
	obs := &fakeObserver{name: "f", deltas: []Delta{{Source: "probe:ci", Kind: "probe_failed", Summary: "ci red", Hints: map[string]string{"severity": "high"}}}}
	e, _ := newEngine(t, Config{Observers: []Observer{obs}, Dial: DialBalanced, Sink: sink})

	e.tickOnce(context.Background())
	e.tickOnce(context.Background()) // identical delta again

	if sink.count() != 1 {
		t.Fatalf("identical repeat should be novelty-suppressed; got %d briefs", sink.count())
	}
}

func TestQuietHoursHoldsNonAlert(t *testing.T) {
	sink := &capturingSink{}
	// notify-level delta during quiet hours → held for digest, not sent now.
	obs := &fakeObserver{name: "f", deltas: []Delta{{Source: "s", Kind: "k", Summary: "fyi", Hints: map[string]string{"severity": "medium"}}}}
	at2am := func() time.Time { return time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC) }
	e, _ := newEngine(t, Config{
		Observers:  []Observer{obs},
		Dial:       DialBalanced,
		Sink:       sink,
		QuietHours: QuietHours{Enabled: true, Start: 22, End: 7},
		Now:        at2am,
	})
	e.tickOnce(context.Background())
	if sink.count() != 0 {
		t.Fatal("quiet hours must hold a notify-level brief")
	}
}

func TestAlertBreaksQuietHours(t *testing.T) {
	sink := &capturingSink{}
	obs := &fakeObserver{name: "f", deltas: []Delta{{Source: "s", Kind: "k", Summary: "PROD DOWN", Hints: map[string]string{"severity": "critical"}}}}
	at2am := func() time.Time { return time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC) }
	e, _ := newEngine(t, Config{
		Observers:  []Observer{obs},
		Sink:       sink,
		QuietHours: QuietHours{Enabled: true, Start: 22, End: 7},
		Now:        at2am,
	})
	e.tickOnce(context.Background())
	if sink.count() != 1 {
		t.Fatal("alert/critical must break quiet hours")
	}
}

func TestDigestFlush(t *testing.T) {
	sink := &capturingSink{}
	obs := &fakeObserver{name: "f", deltas: []Delta{{Source: "s", Kind: "k", Summary: "low item", Hints: map[string]string{"severity": "low"}}}}
	// low → digest under balanced; DigestEvery=1 flushes each beat.
	e, j := newEngine(t, Config{Observers: []Observer{obs}, Dial: DialBalanced, Sink: sink, DigestEvery: 1})
	e.tickOnce(context.Background())
	if sink.count() != 1 {
		t.Fatalf("digest should flush one combined brief, got %d", sink.count())
	}
	if countKind(t, j, event.KindBriefingSent) != 1 {
		t.Fatal("digest flush should emit briefing.sent")
	}
}

func TestPauseResume(t *testing.T) {
	e, j := newEngine(t, Config{})
	e.Pause()
	if !e.IsPaused() {
		t.Fatal("should be paused")
	}
	e.Resume()
	if e.IsPaused() {
		t.Fatal("should be resumed")
	}
	if countKind(t, j, event.KindPulsePaused) != 1 || countKind(t, j, event.KindPulseResumed) != 1 {
		t.Fatal("pause/resume must be journaled")
	}
}

func TestStatusSnapshot(t *testing.T) {
	obs := &fakeObserver{name: "probe:ci"}
	e, _ := newEngine(t, Config{Observers: []Observer{obs}, Dial: DialChatty, Cadence: 5 * time.Second})
	e.tickOnce(context.Background())
	st := e.Status()
	if st.Beats != 1 || st.Dial != "chatty" || len(st.Observers) != 1 || !st.Running {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestObserverErrorJournaledNotFatal(t *testing.T) {
	obs := &fakeObserver{name: "boom", err: context.DeadlineExceeded}
	e, j := newEngine(t, Config{Observers: []Observer{obs}})
	e.tickOnce(context.Background()) // must not panic
	if countKind(t, j, event.KindPulseTick) != 1 {
		t.Fatal("tick should still be emitted despite observer error")
	}
	if countKind(t, j, event.KindObserverDelta) != 1 {
		t.Fatal("observer error should be journaled as an observer.delta carrying the error")
	}
}

func TestStartStopsOnContextCancel(t *testing.T) {
	e, _ := newEngine(t, Config{Cadence: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	e.Start(ctx)
	// Wait until it has actually beat at least once, rather than assuming a fixed
	// sleep is long enough: a loaded Windows CI runner can starve the ticker
	// goroutine past any short fixed sleep (the flake). Poll with a generous
	// deadline — a running engine beats within a cadence or two.
	beatDeadline := time.Now().Add(2 * time.Second)
	for e.Status().Beats == 0 && time.Now().Before(beatDeadline) {
		time.Sleep(e.cadence)
	}
	if e.Status().Beats == 0 {
		t.Fatal("engine never beat before cancel")
	}
	cancel()

	// Wait for the loop goroutine to actually quiesce rather than assuming a
	// fixed sleep is enough — under a loaded parallel test run, cancel
	// propagation (and any in-flight tick) can outlast a short sleep, which is
	// what made this test flaky. A cancelled engine stops within one cadence; a
	// still-running one keeps incrementing. Poll until beats are stable across a
	// window wider than the cadence, then confirm they stay frozen.
	window := 4 * e.cadence
	deadline := time.Now().Add(2 * time.Second)
	var before int64
	stable := false
	for time.Now().Before(deadline) {
		before = e.Status().Beats
		time.Sleep(window)
		if e.Status().Beats == before {
			stable = true
			break
		}
	}
	if !stable {
		t.Fatal("beats never stopped advancing after ctx cancel")
	}
	// Frozen for good: a longer wait yields no further beats.
	time.Sleep(window)
	if got := e.Status().Beats; got != before {
		t.Fatalf("beats advanced from %d to %d after the engine should have stopped", before, got)
	}
}

// panicObserver panics on Poll — verifies the pulse loop contains an observer panic
// instead of crashing the daemon (M423).
type panicObserver struct{ name string }

func (p *panicObserver) Name() string                          { return p.name }
func (p *panicObserver) Poll(context.Context) ([]Delta, error) { panic("observer boom") }

// TestTickOnce_ContainsObserverPanic: a panicking observer must not crash the daemon
// (the pulse loop is a single resident goroutine with no recovering frame); the tick
// completes and the panic is journaled (M423).
func TestTickOnce_ContainsObserverPanic(t *testing.T) {
	e, j := newEngine(t, Config{Observers: []Observer{&panicObserver{name: "boom"}}})
	// Synchronous: without safePoll's recover this panics the test goroutine.
	e.tickOnce(context.Background())
	if countKind(t, j, event.KindPulseTick) != 1 {
		t.Error("tick should still complete despite a panicking observer")
	}
	found := false
	_ = j.Range(func(ev *event.Event) error {
		if ev.Kind == event.KindObserverDelta && strings.Contains(string(ev.Payload), "panic (contained)") {
			found = true
		}
		return nil
	})
	if !found {
		t.Error("a contained observer panic should be journaled")
	}
}
