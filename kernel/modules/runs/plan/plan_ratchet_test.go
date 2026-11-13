// SPDX-License-Identifier: MIT

package plan

// The plan runner's own door (W3.76ak).
//
// A plan is the runs module's own work unit: the scheduler that executes it
// and the live-run registry that holds its cancel are both the module's, so
// the plan runner is the module's service and the daemon hands its face over.
//
// These tests pin that: the service holds the body, it answers in the
// module's own names, and it never names the daemon.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestThePlanRunnerHoldsTheBody pins the BODY: the plan's registration and
// its execution live here, in the module.
func TestThePlanRunnerHoldsTheBody(t *testing.T) {
	src, err := os.ReadFile("plan.go")
	if err != nil {
		t.Fatalf("read plan.go: %v", err)
	}
	text := string(src)
	for _, required := range []string{
		"s.registry.Register(",
		"s.registry.Unregister(",
		"s.registry.Halted()",
		"s.scheduler.Run(",
		"state.ErrHalted",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("the plan runner must hold %s — the plan runner is the module's (W3.76ak)", required)
		}
	}
}

// TestThePlanRunnerAnswersInTheModulesNames pins the FACE: the service
// answers the module's own port, not the daemon's verbs.
func TestThePlanRunnerAnswersInTheModulesNames(t *testing.T) {
	src, err := os.ReadFile("plan.go")
	if err != nil {
		t.Fatalf("read plan.go: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "var _ api.PlanService = (*Service)(nil)") {
		t.Error("the plan runner must answer the module's own port (api.PlanService) — the daemon's verbs are gone (W3.76ak)")
	}
}

// TestThePlanRunnerNeverNamesTheDaemon pins the DIRECTION: the module does
// not import the host. The plan runner is the module's own.
func TestThePlanRunnerNeverNamesTheDaemon(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the plan runner's package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if strings.Contains(string(src), `"github.com/agezt/agezt/kernel/runtime`) {
			t.Errorf("%s imports the daemon — the plan runner is the module's own (W3.76ak)", e.Name())
		}
	}
}
