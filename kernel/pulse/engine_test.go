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

// countSubject counts events of a kind whose Subject matches exactly.
func countSubject(t *testing.T, j *journal.Journal, k event.Kind, subject string) int {
	t.Helper()
	n := 0
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == k && e.Subject == subject {
			n++
		}
		return nil
	})
	return n
}

// TestInitiativeActEmission verifies the M999 actionable-initiative event: an
// actionable (high-severity) observation emits pulse.initiative.act under `act`,
// pulse.initiative.ask under `ask`, and nothing under `off`; a non-actionable
// observation never emits one regardless of level.
func TestInitiativeActEmission(t *testing.T) {
	highDelta := Delta{Source: "probe:ci", Kind: "probe_failed", Summary: "ci failed", Hints: map[string]string{"severity": "high"}}
	medDelta := Delta{Source: "probe:ci", Kind: "probe_flaky", Summary: "ci flaky", Hints: map[string]string{"severity": "medium"}}

	cases := []struct {
		name     string
		level    InitiativeLevel
		delta    Delta
		wantSubj string // "" = expect no initiative.act event
	}{
		{"act + actionable → act event", InitiativeAct, highDelta, "pulse.initiative.act"},
		{"ask + actionable → ask event", InitiativeAsk, highDelta, "pulse.initiative.ask"},
		{"off + actionable → none", InitiativeOff, highDelta, ""},
		{"act + non-actionable → none", InitiativeAct, medDelta, ""},
	}
	for _, c := range cases {
		obs := &fakeObserver{name: "fake", deltas: []Delta{c.delta}}
		e, j := newEngine(t, Config{Observers: []Observer{obs}, Dial: DialBalanced, Initiative: c.level, Sink: &capturingSink{}})
		e.tickOnce(context.Background())

		total := countKind(t, j, event.KindInitiativeAct)
		if c.wantSubj == "" {
			if total != 0 {
				t.Errorf("%s: expected no initiative.act event, got %d", c.name, total)
			}
			continue
		}
		if total != 1 || countSubject(t, j, event.KindInitiativeAct, c.wantSubj) != 1 {
			t.Errorf("%s: want exactly one %s event, got total=%d", c.name, c.wantSubj, total)
		}
		// The per-delta summary event stays exactly one (not double-counted).
		if got := countKind(t, j, event.KindInitiativeTaken); got != 1 {
			t.Errorf("%s: initiative.taken should stay 1, got %d", c.name, got)
		}
	}
}

// TestPendingAsksLifecycle verifies the M1001 ask queue: actionable observations
// under initiative=ask are queued for the operator (not acted), rejection just drops
// one, and approval re-emits the other onto pulse.initiative.act (the act path). Two
// distinct deltas are used so novelty suppression doesn't fold them together.
func TestPendingAsksLifecycle(t *testing.T) {
	d1 := Delta{Source: "probe:ci", Kind: "probe_failed", Summary: "ci failed", Hints: map[string]string{"severity": "high"}}
	d2 := Delta{Source: "probe:disk", Kind: "low_space", Summary: "disk low", Hints: map[string]string{"severity": "high"}}
	obs := &fakeObserver{name: "fake", deltas: []Delta{d1, d2}}
	e, j := newEngine(t, Config{Observers: []Observer{obs}, Dial: DialBalanced, Initiative: InitiativeAsk, Sink: &capturingSink{}})

	e.tickOnce(context.Background())

	// Both asks are queued; the act subject was NOT fired (ask ≠ act).
	asks := e.PendingAsks()
	if len(asks) != 2 {
		t.Fatalf("want 2 pending asks, got %d", len(asks))
	}
	if got := countSubject(t, j, event.KindInitiativeAct, "pulse.initiative.act"); got != 0 {
		t.Fatalf("ask mode must not fire the act subject, got %d", got)
	}
	k0, _ := asks[0]["issue_key"].(string)
	k1, _ := asks[1]["issue_key"].(string)
	if k0 == "" || k1 == "" || k0 == k1 {
		t.Fatalf("expected two distinct issue_keys, got %q and %q", k0, k1)
	}

	// Reject one: drops it, emits no act event, leaves the other queued.
	if found, acted := e.ResolveAsk(k0, false); !found || acted {
		t.Fatalf("reject: found=%v acted=%v, want true,false", found, acted)
	}
	if len(e.PendingAsks()) != 1 {
		t.Fatalf("reject should leave exactly one queued, got %d", len(e.PendingAsks()))
	}
	if got := countSubject(t, j, event.KindInitiativeAct, "pulse.initiative.act"); got != 0 {
		t.Fatalf("reject must not emit an act event, got %d", got)
	}

	// Approve the other: re-emits onto the act subject, queue empties.
	if found, acted := e.ResolveAsk(k1, true); !found || !acted {
		t.Fatalf("approve: found=%v acted=%v, want true,true", found, acted)
	}
	if len(e.PendingAsks()) != 0 {
		t.Fatalf("approve should empty the queue")
	}
	if got := countSubject(t, j, event.KindInitiativeAct, "pulse.initiative.act"); got != 1 {
		t.Fatalf("approve should emit exactly one act event, got %d", got)
	}

	// A second resolve of the same key is a no-op (already gone).
	if found, _ := e.ResolveAsk(k1, true); found {
		t.Fatal("resolving an already-resolved ask should report not found")
	}
}

// TestSetInitiativeLive verifies the live setter normalizes and applies.
func TestSetInitiativeLive(t *testing.T) {
	e, _ := newEngine(t, Config{Initiative: InitiativeAct})
	if got := e.SetInitiative("off"); got != "off" {
		t.Errorf("SetInitiative(off) = %q", got)
	}
	if got := e.SetInitiative("nonsense"); got != "act" {
		t.Errorf("SetInitiative(nonsense) should normalize to act, got %q", got)
	}
}

// --- tests ----------------------------------------------------------------

// waitForTicks polls until at least n pulse.tick events are journaled or the
// deadline passes — used to observe Beat() driving an async tick on the Start loop.
func waitForTicks(t *testing.T, j *journal.Journal, n int) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := countKind(t, j, event.KindPulseTick); got >= n {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	return countKind(t, j, event.KindPulseTick)
}

// TestBeatTriggersOnDemandTick: with a long cadence (so the periodic ticker never
// fires in the test window), Beat() alone drives a heartbeat on the Start loop (M756).
func TestBeatTriggersOnDemandTick(t *testing.T) {
	obs := &fakeObserver{name: "f", deltas: []Delta{{Source: "s", Kind: "k", Summary: "x", Hints: map[string]string{"severity": "high"}}}}
	e, j := newEngine(t, Config{Observers: []Observer{obs}, Cadence: time.Hour, Sink: &capturingSink{}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.Start(ctx)

	if got := countKind(t, j, event.KindPulseTick); got != 0 {
		t.Fatalf("expected 0 ticks before Beat, got %d", got)
	}
	e.Beat()
	if got := waitForTicks(t, j, 1); got < 1 {
		t.Fatalf("expected >=1 tick after Beat, got %d", got)
	}
}

// TestBeatFiresEvenWhenPaused: an explicit Beat() is an operator override that runs
// one heartbeat even while the cadence is paused (M756).
func TestBeatFiresEvenWhenPaused(t *testing.T) {
	obs := &fakeObserver{name: "f"}
	e, j := newEngine(t, Config{Observers: []Observer{obs}, Cadence: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.Start(ctx)
	e.Pause()

	e.Beat()
	if got := waitForTicks(t, j, 1); got < 1 {
		t.Fatalf("Beat() should fire a tick even when paused, got %d ticks", got)
	}
}

// TestSetCadenceChangesAndClamps: SetCadence updates the interval Status reports and
// clamps out-of-range values to [minCadence, maxCadence] (M757).
func TestSetCadenceChangesAndClamps(t *testing.T) {
	e, _ := newEngine(t, Config{Cadence: time.Hour})
	if got := e.Status().CadenceMS; got != time.Hour.Milliseconds() {
		t.Fatalf("initial cadence: %d", got)
	}
	if applied := e.SetCadence(30 * time.Second); applied != 30*time.Second {
		t.Fatalf("applied %v", applied)
	}
	if got := e.Status().CadenceMS; got != 30_000 {
		t.Fatalf("Status should reflect the new cadence, got %d", got)
	}
	if e.SetCadence(time.Millisecond) != minCadence {
		t.Fatal("a too-small cadence should clamp to minCadence")
	}
	if e.SetCadence(48*time.Hour) != maxCadence {
		t.Fatal("a too-large cadence should clamp to maxCadence")
	}
}

// TestSetDialChangesAndNormalizes: SetDial updates the dial Status reports and
// normalizes an unknown value to balanced (M758).
func TestSetDialChangesAndNormalizes(t *testing.T) {
	e, _ := newEngine(t, Config{Dial: DialBalanced})
	if applied := e.SetDial("chatty"); applied != "chatty" {
		t.Fatalf("applied %q", applied)
	}
	if got := e.Status().Dial; got != "chatty" {
		t.Fatalf("Status should report the new dial, got %q", got)
	}
	if applied := e.SetDial("nonsense"); applied != "balanced" {
		t.Fatalf("an unknown dial should normalize to balanced, got %q", applied)
	}
	if got := e.Status().Dial; got != "balanced" {
		t.Fatalf("Status dial after normalize: %q", got)
	}
}

// TestSetQuietHoursChangesAndDisables: SetQuietHours (M770) sets a live window Status
// reports, round-trips the canonical spec, and disables on an empty/invalid value.
func TestSetQuietHoursChangesAndDisables(t *testing.T) {
	e, _ := newEngine(t, Config{})
	if applied := e.SetQuietHours("22-7"); applied != "22-7" {
		t.Fatalf("applied %q, want 22-7", applied)
	}
	q := e.Status().Quiet
	if !q.Enabled || q.Start != 22 || q.End != 7 {
		t.Fatalf("Status quiet = %+v, want enabled 22-7", q)
	}
	// An empty spec disables it.
	if applied := e.SetQuietHours(""); applied != "" {
		t.Fatalf("empty spec should disable, applied %q", applied)
	}
	if e.Status().Quiet.Enabled {
		t.Fatal("quiet hours should be disabled after an empty spec")
	}
	// An invalid spec also disables (ParseQuietHours rejects out-of-range).
	if applied := e.SetQuietHours("99-7"); applied != "" {
		t.Fatalf("invalid spec should disable, applied %q", applied)
	}
}

// TestFlushDigestDeliversAndClears: FlushDigest (M761) composes + delivers the held
// items, clears the digest, journals a briefing, and returns the count; an empty
// flush is a no-op returning 0.
func TestFlushDigestDeliversAndClears(t *testing.T) {
	sink := &capturingSink{}
	e, j := newEngine(t, Config{Sink: sink})
	// Seed the digest directly (white-box) — two held briefs.
	e.digest = []Brief{{Title: "a", Body: "x"}, {Title: "b", Body: "y"}}

	if n := e.FlushDigest(); n != 2 {
		t.Fatalf("expected 2 flushed, got %d", n)
	}
	if len(e.digest) != 0 {
		t.Fatalf("digest should be cleared, got %d", len(e.digest))
	}
	if sink.count() != 1 {
		t.Fatalf("flush should deliver one composed digest brief, got %d", sink.count())
	}
	if countKind(t, j, event.KindBriefingSent) != 1 {
		t.Fatal("flush should journal exactly one briefing.sent")
	}
	if n := e.FlushDigest(); n != 0 {
		t.Fatalf("flushing an empty digest should return 0, got %d", n)
	}
}

// TestAddObserverIsPolledNextBeat: a runtime-added observer (M767) appears in Status
// and is polled on the next beat.
func TestAddObserverIsPolledNextBeat(t *testing.T) {
	e, j := newEngine(t, Config{}) // no initial observers
	obs := &fakeObserver{name: "disk:/test", deltas: []Delta{{Source: "s", Kind: "k", Summary: "x", Hints: map[string]string{"severity": "high"}}}}

	if name := e.AddObserver(obs); name != "disk:/test" {
		t.Fatalf("AddObserver returned %q", name)
	}
	if got := e.Status().Observers; len(got) != 1 || got[0] != "disk:/test" {
		t.Fatalf("Status observers = %v, want [disk:/test]", got)
	}
	e.tickOnce(context.Background())
	if countKind(t, j, event.KindSalienceScored) < 1 {
		t.Fatal("the runtime-added observer should have been polled on the next beat")
	}
}

// TestRemoveObserverDropsOnlyRuntimeAdded (M769): a runtime-added watch can be removed
// by name, while a startup observer with the SAME name is protected. The removable set
// is reported in Status so the UI knows which observers offer a remove control.
func TestRemoveObserverDropsOnlyRuntimeAdded(t *testing.T) {
	// A startup observer named "self:health" (permanent) plus, later, a runtime one that
	// collides on the name — only the runtime one may be removed.
	startup := &fakeObserver{name: "self:health"}
	e, _ := newEngine(t, Config{Observers: []Observer{startup}})

	added := e.AddObserver(&fakeObserver{name: "probe:ci"})
	if added != "probe:ci" {
		t.Fatalf("AddObserver returned %q", added)
	}
	collide := &fakeObserver{name: "self:health"} // same name as the startup observer
	e.AddObserver(collide)

	// Status reports both observers as removable, but the permanent startup one is NOT in
	// the removable set even though it shares a name with the runtime collider.
	s := e.Status()
	if len(s.Observers) != 3 {
		t.Fatalf("expected 3 observers, got %v", s.Observers)
	}
	if !contains(s.Removable, "probe:ci") || !contains(s.Removable, "self:health") {
		t.Fatalf("removable should list both runtime-added names, got %v", s.Removable)
	}

	// Removing "probe:ci" drops exactly one.
	if n := e.RemoveObserver("probe:ci"); n != 1 {
		t.Fatalf("RemoveObserver(probe:ci) = %d, want 1", n)
	}

	// Removing "self:health" drops ONLY the runtime collider, never the startup observer.
	if n := e.RemoveObserver("self:health"); n != 1 {
		t.Fatalf("RemoveObserver(self:health) = %d, want 1 (the runtime collider only)", n)
	}
	after := e.Status()
	if len(after.Observers) != 1 || after.Observers[0] != "self:health" {
		t.Fatalf("the permanent self:health observer must survive, got %v", after.Observers)
	}
	if len(after.Removable) != 0 {
		t.Fatalf("nothing removable should remain, got %v", after.Removable)
	}

	// Removing a name that matches only a permanent observer is a no-op.
	if n := e.RemoveObserver("self:health"); n != 0 {
		t.Fatalf("removing a permanent-only name should drop nothing, got %d", n)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

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
