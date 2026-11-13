// SPDX-License-Identifier: MIT

package api

import (
	"context"

	"github.com/agezt/agezt/kernel/scheduler"
)

// PlanService is the runs module's PLAN RUNNER surface: the plan the
// operator submits, the loop its nodes run, and the registration that makes
// the plan a live run.
//
// The verbs are the MODULE's own names (W3.76ak). The daemon used to answer
// them under names of its own — `RunPlan` and `LoopRunner` — while the
// machinery behind them was already the module's: the SCHEDULER that executes
// a plan is the runs module's family (the architecture gate places
// `kernel/scheduler` in the runs module), and the registration that makes a
// plan a live run is over the module's own live-run registry
// (`kernel/modules/runs/registry`). The daemon held a second handle to both
// and spoke for the family in its own words.
//
// The face carries the plan runner's own operations, not the host's identity:
// the plan id a plan runs under is minted by the CALLER (the application's
// request id), and the plan is executed under an id the caller already holds.
type PlanService interface {
	// Run executes one plan under the plan id and returns the plan's result.
	// An empty plan id asks the module to mint one.
	Run(ctx context.Context, plan scheduler.Plan, planID string) (*scheduler.PlanResult, error)
	// LoopRunner returns the hook that runs one plan node's loop through the
	// run engine.
	LoopRunner() scheduler.LoopRunner
}
