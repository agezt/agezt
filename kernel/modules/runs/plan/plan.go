// SPDX-License-Identifier: MIT

// The runs module's PLAN RUNNER (W3.76ak).
//
// A plan is the runs module's own work unit: the operator submits a plan, the
// scheduler executes its nodes, and the plan is a LIVE RUN while it runs — the
// module's registry holds its cancel, the halt latch refuses new plans, and
// the plan's id is its correlation.
//
// The daemon used to answer for the family in its own words: `RunPlan`
// executed the plan and `LoopRunner` handed the scheduler the hook that runs
// a node's loop. Both bodies were the module's — the SCHEDULER
// (`kernel/scheduler`, the runs module's family per the architecture gate)
// and the module's own live-run REGISTRY (`kernel/modules/runs/registry`) —
// so the daemon held a second handle to the module's machinery and spoke for
// it. The plan runner lives here now.

package plan

import (
	"context"

	"github.com/agezt/agezt/kernel/modules/runs/api"
	"github.com/agezt/agezt/kernel/modules/runs/registry"
	"github.com/agezt/agezt/kernel/modules/runs/state"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/ulid"
)

// Runner is the narrow face of the run engine the plan runner needs: the
// hook that runs one plan node's loop as a run.
//
// It is the module's own engine (`api.RunnerService`), named by the one
// operation the plan runner uses.
type Runner interface {
	// RunWith runs one run under the correlation and returns its answer.
	RunWith(ctx context.Context, corr, intent string) (string, error)
}

// Dependencies are the plan runner's collaborators. Every one of them is the
// runs module's own object: the live-run REGISTRY (the plan's registration
// and the halt latch), the SCHEDULER (the plan's execution) and the run
// ENGINE (the loop a node runs).
type Dependencies struct {
	// Registry is the module's live-run registry: the plan registers here
	// while it runs, and the halt latch lives here too.
	Registry *registry.Registry
	// Scheduler executes the plan's nodes.
	Scheduler *scheduler.Executor
	// Runner runs one plan node's loop as a run.
	Runner Runner
}

// Service is the plan runner: it executes a plan under the module's own
// registry and scheduler.
type Service struct {
	registry  *registry.Registry
	scheduler *scheduler.Executor
	runner    Runner
}

// New builds the plan runner over the module's collaborators.
func New(deps Dependencies) *Service {
	return &Service{registry: deps.Registry, scheduler: deps.Scheduler, runner: deps.Runner}
}

// Run executes one plan under the plan id.
//
// The plan is a LIVE RUN while it executes: it registers its cancel in the
// module's registry, so a halt cancels it like any other run, and it
// unregisters when it is done.
//
// A halted daemon refuses the plan — the same refusal every run gets
// (`state.ErrHalted`, the module's own sentinel: the daemon's alias points at
// the same value).
//
// An empty plan id asks the module to mint one (`plan-<ulid>`).
func (s *Service) Run(ctx context.Context, pl scheduler.Plan, planID string) (*scheduler.PlanResult, error) {
	s.registry.Lock()
	if s.registry.Halted() {
		s.registry.Unlock()
		return nil, state.ErrHalted
	}
	if planID == "" {
		planID = "plan-" + ulid.New()
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.registry.Register(planID, cancel)
	s.registry.Unlock()

	defer func() {
		s.registry.Lock()
		s.registry.Unregister(planID)
		s.registry.Unlock()
		cancel()
	}()

	return s.scheduler.Run(runCtx, pl, planID)
}

// LoopRunner returns the hook that runs one plan node's loop through the run
// engine.
//
// The scheduler's hook takes the intent and the correlation id in that order;
// the run engine takes the correlation first. The mapping is the plan
// runner's, because it is the one that holds both.
func (s *Service) LoopRunner() scheduler.LoopRunner {
	return func(ctx context.Context, intent, corr string) (string, error) {
		return s.runner.RunWith(ctx, corr, intent)
	}
}

var _ api.PlanService = (*Service)(nil)
