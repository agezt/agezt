// SPDX-License-Identifier: MIT

package reflect

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/worldmodel"
)

var fixedNow = time.Unix(1_700_000_000, 0).UTC()

const day = 24 * 60 * 60 * 1000

func newTestEngine(t *testing.T, cfg Config) (*Engine, *bus.Bus, *worldmodel.Graph, *journal.Journal) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	b := bus.New(j)
	ws, err := worldmodel.Open(t.TempDir())
	if err != nil {
		t.Fatalf("worldmodel.Open: %v", err)
	}
	w := worldmodel.NewGraph(ws, b)
	e := New(j, w, b, cfg)
	e.now = func() time.Time { return fixedNow }
	t.Cleanup(func() { b.Close(); j.Close() })
	return e, b, w, j
}

func emit(t *testing.T, b *bus.Bus, kind event.Kind, n int) {
	t.Helper()
	for range n {
		if _, err := b.Publish(event.Spec{Subject: string(kind), Kind: kind, Actor: "test"}); err != nil {
			t.Fatalf("publish %s: %v", kind, err)
		}
	}
}

func TestObserveFoldsCounts(t *testing.T) {
	e, b, _, _ := newTestEngine(t, Config{})
	emit(t, b, event.KindTaskReceived, 5)
	emit(t, b, event.KindTaskCompleted, 3) // 2 failed (incomplete)
	emit(t, b, event.KindBriefingSent, 2)
	emit(t, b, event.KindApprovalDenied, 4)
	emit(t, b, event.KindApprovalGranted, 1)
	emit(t, b, event.KindSkillActivated, 2)

	rep, err := e.Reflect(context.Background(), "r1")
	if err != nil {
		t.Fatalf("reflect: %v", err)
	}
	o := rep.Observations
	if o.TasksStarted != 5 || o.TasksCompleted != 3 || o.TasksFailed != 2 {
		t.Errorf("task counts wrong: %+v", o)
	}
	if o.BriefsSent != 2 || o.ApprovalsDenied != 4 || o.ApprovalsGranted != 1 || o.SkillsActivated != 2 {
		t.Errorf("counts wrong: %+v", o)
	}
}

func TestReflectRunsDecayAndJournals(t *testing.T) {
	// The decay arithmetic is covered in worldmodel/decay_test; here we just
	// assert the reflection pass invokes it and journals its report. A fresh
	// entity is not stale, so 0 decayed.
	e, _, w, j := newTestEngine(t, Config{Decay: worldmodel.DecayOptions{StaleAfterMS: day, Factor: 0.5}})
	if _, _, err := w.Upsert("seed", worldmodel.UpsertSpec{Kind: worldmodel.KindProject, Name: "fresh"}); err != nil {
		t.Fatal(err)
	}
	rep, err := e.Reflect(context.Background(), "r1")
	if err != nil {
		t.Fatalf("reflect: %v", err)
	}
	if rep.EntitiesDecayed != 0 {
		t.Errorf("fresh entity → 0 decayed, got %d", rep.EntitiesDecayed)
	}
	if rep.Observations.EntitiesTotal != 1 {
		t.Errorf("entities_total = %d want 1", rep.Observations.EntitiesTotal)
	}
	if countKind(t, j, event.KindReflectionCompleted) != 1 {
		t.Error("reflect must journal reflection.completed")
	}
}

func TestProposalsFireAtThresholds(t *testing.T) {
	e, b, _, _ := newTestEngine(t, Config{BriefVolume: 3, DenyExcess: 2})
	emit(t, b, event.KindBriefingSent, 3)   // hits pulse rule
	emit(t, b, event.KindApprovalDenied, 3) // 3-0 >= 2 → autonomy rule
	emit(t, b, event.KindTaskReceived, 4)
	emit(t, b, event.KindTaskCompleted, 1) // 3 of 4 failed → tasks rule

	rep, _ := e.Reflect(context.Background(), "r1")
	areas := map[string]bool{}
	for _, p := range rep.Proposals {
		areas[p.Area] = true
	}
	for _, want := range []string{"pulse", "autonomy", "tasks"} {
		if !areas[want] {
			t.Errorf("expected a %q proposal; got %+v", want, rep.Proposals)
		}
	}
}

// TestProposals_ExactThresholds pins the inclusive boundary of the two rules that the
// existing tests only exercise clear of their thresholds. TestProposalsFireAtThresholds
// fires the autonomy rule with a denied-granted excess of 3 against DenyExcess 2, and the
// tasks rule at 75% failure — both well past the `>=` edge — so mutation testing (M520)
// left `>= → >` alive on `ApprovalsDenied-ApprovalsGranted >= denyExcess` and on
// `TasksFailed*2 >= TasksStarted` (the ≥50%-failure rule). Each rule must fire at exactly
// its threshold and stay silent one step below.
func TestProposals_ExactThresholds(t *testing.T) {
	e := &Engine{cfg: Config{BriefVolume: 8, DenyExcess: 3}}
	has := func(ps []Proposal, area string) bool {
		for _, p := range ps {
			if p.Area == area {
				return true
			}
		}
		return false
	}

	// Autonomy: denied-granted >= denyExcess(3). Exactly 3 fires; 2 is silent.
	if !has(e.proposals(Observations{ApprovalsDenied: 5, ApprovalsGranted: 2}), "autonomy") {
		t.Error("autonomy: denied-granted == denyExcess (3) must fire")
	}
	if has(e.proposals(Observations{ApprovalsDenied: 4, ApprovalsGranted: 2}), "autonomy") {
		t.Error("autonomy: denied-granted == denyExcess-1 (2) must be silent")
	}

	// Tasks: failed*2 >= started (a ≥50% failure rate). Exactly 50% fires; under doesn't.
	if !has(e.proposals(Observations{TasksFailed: 2, TasksStarted: 4}), "tasks") {
		t.Error("tasks: exactly 50% failure (failed*2 == started) must fire")
	}
	if has(e.proposals(Observations{TasksFailed: 2, TasksStarted: 5}), "tasks") {
		t.Error("tasks: under 50% (failed*2 < started) must be silent")
	}
}

func TestProposalsSilentBelowThreshold(t *testing.T) {
	e, b, _, _ := newTestEngine(t, Config{})
	emit(t, b, event.KindBriefingSent, 1)
	emit(t, b, event.KindTaskReceived, 2)
	emit(t, b, event.KindTaskCompleted, 2)
	rep, _ := e.Reflect(context.Background(), "r1")
	if len(rep.Proposals) != 0 {
		t.Errorf("quiet window should yield no proposals, got %+v", rep.Proposals)
	}
}

func TestLatestReturnsNewest(t *testing.T) {
	e, _, _, _ := newTestEngine(t, Config{})
	if _, ok := e.Latest(); ok {
		t.Error("no reflection yet → Latest should be false")
	}
	_, _ = e.Reflect(context.Background(), "r1")
	_, _ = e.Reflect(context.Background(), "r2")
	rep, ok := e.Latest()
	if !ok {
		t.Fatal("Latest should find a report")
	}
	if rep.GeneratedMS != fixedNow.UnixMilli() {
		t.Errorf("latest report timestamp = %d", rep.GeneratedMS)
	}
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
