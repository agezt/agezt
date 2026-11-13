// SPDX-License-Identifier: MIT

package runner

// The run engine's RUN'S MODEL RESOLUTION door (W3.76al).
//
// A run asks for a model: a per-run pick wins, and it is reported as
// EXPLICIT — otherwise the daemon's effective default answers.
//
// The per-run pick is the RUN CONTEXT's own vocabulary, and the engine — which
// lives inside the module — used to ask the HOST to apply it: a round trip
// inside one module for a decision the run context already holds.
//
// These tests pin that: the port carries the host's DEFAULT and no rule, and
// the engine applies the rule itself.

import (
	"os"
	"regexp"
	"testing"
)

// theModelResolutionEntriesAreNotTheHosts are the model's resolution entries
// the run engine's host port must NOT carry: the rule is the run context's.
var theModelResolutionEntriesAreNotTheHosts = regexp.MustCompile(`(?m)^\tResolveRunModel\(`)

// TestTheRunEnginePortCarriesNoModelResolution pins the PORT: it promises the
// host's state, not the module's own rule.
func TestTheRunEnginePortCarriesNoModelResolution(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("read api.go: %v", err)
	}
	if theModelResolutionEntriesAreNotTheHosts.Match(src) {
		t.Error("the run engine's port carries a model-resolution entry — the resolution is the run context's (W3.76al)")
	}
}

// TestTheRunEngineAsksTheRunContextForTheModelRule pins the WIRING: the
// engine applies the rule itself and asks the host only for the default.
func TestTheRunEngineAsksTheRunContextForTheModelRule(t *testing.T) {
	if !runnerPackageContains(t, "runctx.ResolveModel(") {
		t.Error("the run engine must ask the run context for the model's rule — the rule is the run context's (W3.76al)")
	}
	if !runnerPackageContains(t, "r.k.EffectiveModel(") {
		t.Error("the run engine must ask the host for the model's DEFAULT — the host owns that state (W3.76al)")
	}
}
