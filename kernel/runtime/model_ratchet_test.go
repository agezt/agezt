// SPDX-License-Identifier: MIT

package runtime

// The daemon's RUN'S MODEL RESOLUTION door (W3.76al).
//
// A run asks for a model: a per-run pick wins, and it is reported as
// EXPLICIT — otherwise the daemon's effective default answers.
//
// The per-run pick is the RUN CONTEXT's own vocabulary (`runctx.WithModel`),
// and the run engine — which lives inside the runs module — used to ask the
// HOST to apply it: a round trip inside one module for a decision the run
// context already holds.
//
// These tests pin that: the daemon holds no verb for the resolution, no copy
// of the rule, and only the host's own state — the effective default.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// theModelResolutionVerbsAreNotTheDaemons are the model's resolution verbs
// the daemon must not hold: the rule is the run context's, and a verb per
// rule is a door the host never had to keep.
var theModelResolutionVerbsAreNotTheDaemons = regexp.MustCompile(`(?m)^func \(k \*Kernel\) ResolveRunModel\(`)

// theModelRuleIsNotTheDaemons is the daemon's private copy of the rule: a
// helper that resolves the model from the run context. The rule lives in the
// run context's own package now.
var theModelRuleIsNotTheDaemons = regexp.MustCompile(`(?m)^func resolveRunModel\(`)

// TestTheDaemonHoldsNoVerbPerModelResolution pins the SURFACE: the daemon
// answers for no model resolution.
func TestTheDaemonHoldsNoVerbPerModelResolution(t *testing.T) {
	for _, f := range daemonProductionFiles(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if theModelResolutionVerbsAreNotTheDaemons.Match(src) {
			t.Errorf("%s declares a model-resolution verb — the resolution is the run context's (W3.76al)", f)
		}
	}
}

// TestTheDaemonHoldsNoCopyOfTheModelRule pins the BODY: the daemon keeps no
// private copy of the rule under another name.
func TestTheDaemonHoldsNoCopyOfTheModelRule(t *testing.T) {
	for _, f := range daemonProductionFiles(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if theModelRuleIsNotTheDaemons.Match(src) {
			t.Errorf("%s grew a copy of the model's rule back — the rule is the run context's (W3.76al)", f)
		}
	}
}

// TestTheDaemonAsksTheModuleForTheModelRule pins the WIRING from the host's
// side: the daemon asks the MODULE for the rule and keeps only its own state.
func TestTheDaemonAsksTheModuleForTheModelRule(t *testing.T) {
	methods := daemonMethodSet(t)
	if !methods["EffectiveModel"] {
		t.Error("*Kernel has no EffectiveModel verb — the daemon keeps the host's own default")
	}
	found := false
	for _, f := range daemonProductionFiles(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if strings.Contains(string(src), "runctx.ResolveModel(") {
			found = true
		}
	}
	if !found {
		t.Error("the daemon must ask the run context's module for the model's rule — the rule is the module's (W3.76al)")
	}
}
