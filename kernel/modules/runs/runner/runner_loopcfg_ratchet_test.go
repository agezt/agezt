// SPDX-License-Identifier: MIT

package runner

// The run engine's LOOP CONFIGURATION door (W3.77).
//
// The shape every governed run's loop must have — root or delegated — is the
// RUNS MODULE's own rule (kernel/modules/runs/loopcfg, W3.76g). The daemon
// held a door over it (BuildLoopConfig) and the run engine asked the HOST for
// it through its port: a round trip inside ONE module, for a rule the module
// already owns.
//
// The engine holds the module's service now — the very same service the
// delegation engine holds — and asks the module.
//
// These tests pin that: the port carries no loop-configuration entry, the
// engine holds the module's service, and a host that wired none leaves the
// engine quiet instead of running an ungoverned loop.

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/modules/runs/loopcfg"
)

// theLoopCfgEntriesAreNotTheHosts are the loop-configuration operations the
// run engine's host port must NOT carry: the shape of a governed run's loop is
// the module's own, and a port entry that is a round trip inside one module is
// a promise the host never had to keep.
var theLoopCfgEntriesAreNotTheHosts = regexp.MustCompile(`(?m)^\t(BuildLoopConfig)\(`)

// TestTheRunEnginePortCarriesNoLoopConfigEntry pins the PORT: it promises the
// host's capabilities, not the module's own loop-configuration rule.
func TestTheRunEnginePortCarriesNoLoopConfigEntry(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("read api.go: %v", err)
	}
	if theLoopCfgEntriesAreNotTheHosts.Match(src) {
		t.Error("the run engine's port carries a loop-configuration entry — the loop configuration is the module's own (W3.77)")
	}
}

// TestTheRunEngineDoesNotAskTheHostForTheLoopConfiguration pins the CALL: the
// engine asks the MODULE for the shape of a run's loop. A host door here
// would be a round trip inside one module — for a rule the module owns.
func TestTheRunEngineDoesNotAskTheHostForTheLoopConfiguration(t *testing.T) {
	src, err := os.ReadFile("runner.go")
	if err != nil {
		t.Fatalf("read runner.go: %v", err)
	}
	if strings.Contains(string(src), "r.k.BuildLoopConfig(") {
		t.Error("the run engine asks the host for the loop configuration — the rule is the module's own (W3.77)")
	}
}

// TestTheRunEngineHoldsTheModulesLoopConfiguration pins the wiring: the engine
// holds the module's service, not a host door.
func TestTheRunEngineHoldsTheModulesLoopConfiguration(t *testing.T) {
	svc := testLoopCfg()
	r := New(&stubKernel{}, nil, testState(), testPrompt(), testPublisher(), testTickets(), svc)
	if r.loopCfg != svc {
		t.Fatal("the run engine does not hold the module's loop configuration")
	}
}

// TestTheRunEngineRefusesWithoutALoopConfiguration pins the refusal: a host
// that wired no loop configuration leaves the engine quiet — it says the
// configuration is missing instead of running an ungoverned loop.
func TestTheRunEngineRefusesWithoutALoopConfiguration(t *testing.T) {
	r := New(&stubKernel{}, nil, testState(), testPrompt(), testPublisher(), testTickets(), nil)
	if _, err := r.RunWith(t.Context(), "corr-1", "do the thing"); err == nil {
		t.Fatal("the engine ran without a loop configuration")
	}
}

// testLoopCfg returns the module's loop configuration over a stub host — the
// smallest configuration a test can build a governed run's loop with.
func testLoopCfg() *loopcfg.Service {
	return loopcfg.New(loopcfg.Deps{
		Config: func(context.Context) loopcfg.Config { return loopcfg.Config{MaxIter: 1} },
		Tools:  func(context.Context) map[string]toolapi.Tool { return map[string]toolapi.Tool{} },
	})
}
