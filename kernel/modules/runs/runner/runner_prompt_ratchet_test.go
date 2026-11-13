// SPDX-License-Identifier: MIT

package runner

// The run engine's PROMPT door (W3.76af).
//
// The prompt assembly — the layer that decides what a run knows before it
// starts — is the RUNS MODULE's own (kernel/modules/runs/prompt). The daemon
// held two verbs over it (BuildRunPrompt, InjectHostEnvironment) and the run
// engine asked the HOST for them through its port: a round trip inside ONE
// module, twice over.
//
// The engine holds the module's service now — the very same service the
// delegation engine holds — and asks the module.
//
// These tests pin that: the port carries no prompt entry, the engine holds
// the module's service, and a host that wired none leaves the engine quiet
// instead of running a bare prompt.

import (
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/prompt"
)

// thePromptEntriesAreNotTheHosts are the prompt-assembly operations the run
// engine's host port must NOT carry: the assembly is the module's own, and a
// port entry that is a round trip inside one module is a promise the host
// never had to keep.
var thePromptEntriesAreNotTheHosts = regexp.MustCompile(`(?m)^\t(BuildRunPrompt|InjectHostEnvironment)\(`)

// TestTheRunEnginePortCarriesNoPromptEntry pins the PORT: it promises the
// host's capabilities, not the module's own prompt assembly.
func TestTheRunEnginePortCarriesNoPromptEntry(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("read api.go: %v", err)
	}
	if thePromptEntriesAreNotTheHosts.Match(src) {
		t.Error("the run engine's port carries a prompt entry — the prompt assembly is the module's own (W3.76af)")
	}
}

// TestTheRunEngineAsksTheModulesPromptAssembly pins the wiring: the engine
// holds the module's service, not a host door.
func TestTheRunEngineAsksTheModulesPromptAssembly(t *testing.T) {
	p := testPrompt()
	r := New(&stubKernel{}, nil, testState(), p, testPublisher(), testTickets(), testLoopCfg())
	if r.prompt != p {
		t.Fatal("the run engine does not hold the module's prompt assembly")
	}
}

// TestTheRunEngineRefusesWithoutAPromptAssembly pins the refusal: a host
// that wired no prompt service leaves the engine quiet — it says the
// assembly is missing instead of running a bare prompt.
func TestTheRunEngineRefusesWithoutAPromptAssembly(t *testing.T) {
	r := New(&stubKernel{}, nil, testState(), nil, testPublisher(), testTickets(), testLoopCfg())
	if _, err := r.RunWith(t.Context(), "corr-1", "do the thing"); err == nil {
		t.Fatal("the engine ran without a prompt assembly")
	}
}

// stubPromptHost is the HOST side of the prompt module's three-verb port:
// the live identity, the bus and the clock. The run engine's tests need no
// bus and no wall clock — they need an answer.
type stubPromptHost struct{}

// System is the daemon's live default identity.
func (stubPromptHost) System() string { return "tester" }

// Publish journals an event; the tests do not read it.
func (stubPromptHost) Publish(spec event.Spec) (*event.Event, error) { return &event.Event{}, nil }

// Now is the host's clock.
func (stubPromptHost) Now() time.Time { return time.Unix(0, 0).UTC() }

// testPrompt returns the module's prompt assembly over a stub host — the
// smallest assembly a test can build a run's prompt with.
func testPrompt() *prompt.Service {
	return prompt.New(prompt.Config{}, stubPromptHost{}, prompt.Deps{})
}
