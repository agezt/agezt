// SPDX-License-Identifier: MIT

package plan

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/modules/runs/registry"
	"github.com/agezt/agezt/kernel/modules/runs/state"
	"github.com/agezt/agezt/kernel/scheduler"
)

// fakeRunner is the run engine's face, reduced to the one operation the plan
// runner uses.
type fakeRunner struct {
	answer string
	err    error
	// seen records the correlation and the intent of the last call.
	corr   string
	intent string
	calls  int
}

func (f *fakeRunner) RunWith(_ context.Context, corr, intent string) (string, error) {
	f.calls++
	f.corr = corr
	f.intent = intent
	return f.answer, f.err
}

// newService builds a plan runner over a real registry, a real scheduler and
// a fake engine.
func newService(t *testing.T) (*Service, *registry.Registry, *fakeRunner) {
	t.Helper()
	dir := t.TempDir()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatalf("open the journal: %v", err)
	}
	t.Cleanup(func() { j.Close() })

	reg := registry.New()
	runner := &fakeRunner{answer: "ok"}
	svc := New(Dependencies{
		Registry:  reg,
		Scheduler: scheduler.New(scheduler.Config{Bus: bus.New(j)}),
		Runner:    runner,
	})
	return svc, reg, runner
}

// oneNodePlan is a plan with a single loop node.
func oneNodePlan(svc *Service) scheduler.Plan {
	return scheduler.Plan{
		Name: "p",
		Nodes: []scheduler.Node{
			&scheduler.LoopNode{NodeID: "n1", Intent: "do it", Runner: svc.LoopRunner()},
		},
	}
}

// TestThePlanRunnerRefusesAHaltedDaemon pins the refusal: a halted daemon
// refuses the plan, exactly as it refuses a run — and it refuses BEFORE the
// plan registers, so a refused plan never becomes a live run.
func TestThePlanRunnerRefusesAHaltedDaemon(t *testing.T) {
	svc, reg, runner := newService(t)
	reg.SetHalted(true)

	res, err := svc.Run(context.Background(), oneNodePlan(svc), "plan-1")
	if !errors.Is(err, state.ErrHalted) {
		t.Fatalf("err = %v, want the module's halted sentinel", err)
	}
	if res != nil {
		t.Errorf("result = %+v, want nil", res)
	}
	if reg.Count() != 0 {
		t.Errorf("the registry holds %d runs, want none — a refused plan never registers", reg.Count())
	}
	if runner.calls != 0 {
		t.Errorf("the engine ran %d times, want none", runner.calls)
	}
}

// TestThePlanRunnerRegistersThePlanWhileItRuns pins the plan's LIVE-RUN
// nature: the plan registers in the module's registry while it executes (so a
// halt cancels it like any other run) and unregisters when it is done.
func TestThePlanRunnerRegistersThePlanWhileItRuns(t *testing.T) {
	svc, reg, runner := newService(t)

	// The engine observes the registry while the plan runs.
	runner.answer = "ok"
	svcRunner := &observingRunner{inner: runner, reg: reg}

	svc2 := New(Dependencies{
		Registry:  reg,
		Scheduler: svc.scheduler,
		Runner:    svcRunner,
	})

	if _, err := svc2.Run(context.Background(), oneNodePlan(svc2), "plan-42"); err != nil {
		t.Fatalf("run the plan: %v", err)
	}
	if !svcRunner.seenRegistered {
		t.Error("the plan was not registered in the module's registry while it ran")
	}
	if reg.Count() != 0 {
		t.Errorf("the registry holds %d runs after the plan finished, want none", reg.Count())
	}
}

// observingRunner watches the registry while the plan runs.
type observingRunner struct {
	inner          *fakeRunner
	reg            *registry.Registry
	seenRegistered bool
}

func (o *observingRunner) RunWith(ctx context.Context, corr, intent string) (string, error) {
	o.seenRegistered = o.reg.Running(corr) || o.reg.Count() > 0
	return o.inner.RunWith(ctx, corr, intent)
}

// TestThePlanRunnerMintsAPlanID pins the id: an empty plan id asks the
// module to mint one, and the minted id names the plan.
func TestThePlanRunnerMintsAPlanID(t *testing.T) {
	svc, _, _ := newService(t)

	res, err := svc.Run(context.Background(), oneNodePlan(svc), "")
	if err != nil {
		t.Fatalf("run the plan: %v", err)
	}
	if !strings.HasPrefix(res.PlanID, "plan-") {
		t.Errorf("plan id = %q, want a minted plan-<ulid>", res.PlanID)
	}
	if res.PlanID == "plan-" {
		t.Error("the minted plan id carries no ulid")
	}
}

// TestThePlanRunnerKeepsTheCallersPlanID pins the other half: the id the
// caller holds is the id the plan runs under.
func TestThePlanRunnerKeepsTheCallersPlanID(t *testing.T) {
	svc, _, _ := newService(t)

	res, err := svc.Run(context.Background(), oneNodePlan(svc), "plan-keep")
	if err != nil {
		t.Fatalf("run the plan: %v", err)
	}
	if res.PlanID != "plan-keep" {
		t.Errorf("plan id = %q, want the caller's id", res.PlanID)
	}
}

// TestTheLoopRunnerRunsTheNodeThroughTheEngine pins the hook's mapping: the
// scheduler's hook takes the intent and the correlation id IN THAT ORDER,
// while the run engine takes the correlation first. The plan runner holds
// both, so it owns the mapping — and a swapped pair would run the node under
// the intent as its correlation.
func TestTheLoopRunnerRunsTheNodeThroughTheEngine(t *testing.T) {
	svc, _, runner := newService(t)

	answer, err := svc.LoopRunner()(context.Background(), "the intent", "the-corr")
	if err != nil {
		t.Fatalf("run the loop: %v", err)
	}
	if answer != "ok" {
		t.Errorf("answer = %q, want the engine's answer", answer)
	}
	if runner.corr != "the-corr" {
		t.Errorf("the engine ran under %q, want the correlation id", runner.corr)
	}
	if runner.intent != "the intent" {
		t.Errorf("the engine ran %q, want the intent", runner.intent)
	}
}
