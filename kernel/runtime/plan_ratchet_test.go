// SPDX-License-Identifier: MIT

package runtime

// The daemon's PLAN RUNNER door (W3.76ak).
//
// A plan is the runs module's own work unit: the operator submits a plan, the
// scheduler executes its nodes, and the plan is a LIVE RUN while it runs —
// the module's registry holds its cancel, the halt latch refuses new plans.
//
// The daemon used to answer for the family in its own words: `RunPlan`
// executed the plan and `LoopRunner` handed the scheduler the hook that runs a
// node's loop. Both bodies were the module's — the SCHEDULER (the runs
// module's family, per the architecture gate) and the module's own live-run
// REGISTRY.
//
// These tests pin that: the daemon holds no verb per plan operation, no plan
// registration of its own, and no execution of its own — it holds the
// module's FACE.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// thePlanVerbsAreNotTheDaemons are the plan runner's verbs the daemon must
// not hold: the plan runner is the module's service, and a verb per plan
// operation is a door the host never had to keep.
var thePlanVerbsAreNotTheDaemons = regexp.MustCompile(`(?m)^func \(k \*Kernel\) (RunPlan|LoopRunner)\(`)

// thePlanRegistrationIsNotTheDaemons is the plan's registration: the plan
// registers in the module's store while it runs, so the daemon must not
// register one itself.
var thePlanRegistrationIsNotTheDaemons = regexp.MustCompile(`k\.registry\.(Register|Unregister)\(`)

// thePlanExecutionIsNotTheDaemons is the plan's execution: the scheduler that
// executes a plan is the module's, so the daemon must not drive it.
var thePlanExecutionIsNotTheDaemons = regexp.MustCompile(`k\.scheduler\.Run\(`)

// TestTheDaemonHoldsNoVerbPerPlanOperation pins the SURFACE: the daemon
// answers for no plan operation.
func TestTheDaemonHoldsNoVerbPerPlanOperation(t *testing.T) {
	for _, f := range daemonProductionFiles(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if thePlanVerbsAreNotTheDaemons.Match(src) {
			t.Errorf("%s declares a plan-runner verb — the plan runner is the module's (W3.76ak)", f)
		}
	}
}

// TestTheDaemonRegistersNoPlan pins the BODY: the daemon registers no plan
// of its own. The plan's registration is the module's, in the module's store.
func TestTheDaemonRegistersNoPlan(t *testing.T) {
	for _, f := range daemonProductionFiles(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if thePlanRegistrationIsNotTheDaemons.Match(src) {
			t.Errorf("%s registers a plan — the plan's registration is the module's (W3.76ak)", f)
		}
	}
}

// TestTheDaemonRunsNoPlan pins the BODY's other half: the daemon drives no
// scheduler. It hands the scheduler to the module and asks the module.
func TestTheDaemonRunsNoPlan(t *testing.T) {
	for _, f := range daemonProductionFiles(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if thePlanExecutionIsNotTheDaemons.Match(src) {
			t.Errorf("%s runs a plan — the plan runner is the module's (W3.76ak)", f)
		}
	}
}

// TestTheDaemonHandsOverTheModulesPlanRunner pins the WIRING from the host's
// side: the daemon holds the module's face and the composition root builds it
// over the module's own registry, scheduler and engine.
func TestTheDaemonHandsOverTheModulesPlanRunner(t *testing.T) {
	methods := daemonMethodSet(t)
	if !methods["PlanService"] {
		t.Error("*Kernel has no PlanService verb — the daemon hands the module's face over")
	}
	src, err := os.ReadFile("compose.go")
	if err != nil {
		t.Fatalf("read compose.go: %v", err)
	}
	text := string(src)
	for _, required := range []string{
		"k.plan = plan.New(",
		"Registry:  k.registry,",
		"Scheduler: k.scheduler,",
		"Runner:    k.runner,",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("the composition root must build the plan runner over the module's own parts (%s) — the plan runner is the module's (W3.76ak)", required)
		}
	}
}
