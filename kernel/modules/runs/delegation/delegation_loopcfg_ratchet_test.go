// SPDX-License-Identifier: MIT

package delegation

// The delegation engine's LOOP CONFIGURATION door (W3.77).
//
// The shape every governed run's loop must have — the child's base,
// identical to a root run's — is the RUNS MODULE's own rule
// (kernel/modules/runs/loopcfg, W3.76g). The daemon held a door over it
// (BaseLoopConfig) and the delegation engine — which lives INSIDE the module
// that owns the rule — asked the host for it: a round trip inside one module.
//
// The engine holds the module's service now — the very same service the run
// engine holds — and asks the module.
//
// These tests pin that: the host port carries no loop-configuration entry,
// the engine holds the module's service, and a host that wired none leaves
// the engine quiet instead of running an ungoverned child.

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/modules/runs/loopcfg"
)

// theLoopCfgDoorsAreNotTheHosts are the loop-configuration operations the
// delegation engine's host port must NOT carry: the shape of a governed run's
// loop is the module's own, and a door that is a round trip inside one module
// is a promise the host never had to keep.
var theLoopCfgDoorsAreNotTheHosts = regexp.MustCompile(`(?m)^\tBaseLoopConfig\(`)

// TestTheDelegationHostPortCarriesNoLoopConfigDoor pins the PORT: it promises
// the host's capabilities, not the module's own loop-configuration rule.
func TestTheDelegationHostPortCarriesNoLoopConfigDoor(t *testing.T) {
	src, err := os.ReadFile("host.go")
	if err != nil {
		t.Fatalf("read host.go: %v", err)
	}
	if theLoopCfgDoorsAreNotTheHosts.Match(src) {
		t.Error("the delegation host port carries a loop-configuration door — the loop configuration is the module's own (W3.77)")
	}
}

// TestTheDelegationEngineAsksTheModulesLoopConfiguration pins the wiring: the
// engine holds the module's service and asks it, not the host.
func TestTheDelegationEngineAsksTheModulesLoopConfiguration(t *testing.T) {
	src, err := os.ReadFile("exec.go")
	if err != nil {
		t.Fatalf("read exec.go: %v", err)
	}
	body := string(src)
	if !strings.Contains(body, "s.deps.LoopCfg.Build(") {
		t.Error("the delegation engine does not ask the module's loop configuration")
	}
	if strings.Contains(body, "s.host.BaseLoopConfig(") {
		t.Error("the delegation engine asks the host for the child's loop configuration — the rule is the module's own")
	}
}

// TestTheDelegationEngineRefusesWithoutALoopConfiguration pins the refusal: a
// host that wired no loop configuration leaves the engine quiet — the child
// does not run an ungoverned loop.
func TestTheDelegationEngineRefusesWithoutALoopConfiguration(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	svc.deps.LoopCfg = nil
	_, err := svc.Run(context.Background(), Request{Task: "t"})
	if err == nil {
		t.Fatal("the engine ran a child without a loop configuration")
	}
	if !strings.Contains(err.Error(), "loop configuration is not available") {
		t.Fatalf("error = %v, want the loop configuration to be missing", err)
	}
}

// testLoopCfg returns the module's loop configuration over a stub host — the
// smallest configuration a test can build a governed child's loop with.
func testLoopCfg() *loopcfg.Service {
	return loopcfg.New(loopcfg.Deps{
		Config: func(context.Context) loopcfg.Config { return loopcfg.Config{MaxIter: 1} },
		Tools:  func(context.Context) map[string]toolapi.Tool { return map[string]toolapi.Tool{} },
	})
}
