// SPDX-License-Identifier: MIT

package resume

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// TestCarriesTicket_ScopeIsTheRootRun: the scope rule. A root run carries its
// own ticket; a delegated (sub-agent) run does not, because its work is
// re-driven by resuming the root.
func TestCarriesTicket_ScopeIsTheRootRun(t *testing.T) {
	for _, tc := range []struct {
		name       string
		parentCorr string
		want       bool
	}{
		{"root run", "", true},
		{"delegated run", "corr-parent", false},
		{"delegated run, other parent", "corr-other", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CarriesTicket(tc.parentCorr); got != tc.want {
				t.Fatalf("CarriesTicket(%q) = %v, want %v", tc.parentCorr, got, tc.want)
			}
		})
	}
}

// TestTerminalOutcome_OnlyAShutdownInterruptionKeepsTheTicket: the termination
// rule. A run cancelled while the daemon is SUSPENDING was interrupted by a
// restart, so its ticket survives; every other terminal outcome deletes it.
func TestTerminalOutcome_OnlyAShutdownInterruptionKeepsTheTicket(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name       string
		runErr     error
		suspending bool
		want       Disposition
	}{
		{"clean finish", nil, false, DispositionDelete},
		{"clean finish while suspending", nil, true, DispositionDelete},
		{"failure", boom, false, DispositionDelete},
		{"failure while suspending", boom, true, DispositionDelete},
		{"operator cancel", context.Canceled, false, DispositionDelete},
		{"operator cancel while suspending", context.Canceled, true, DispositionKeep},
		{"timeout", context.DeadlineExceeded, false, DispositionDelete},
		{"timeout while suspending", context.DeadlineExceeded, true, DispositionKeep},
		{"wrapped cancel while suspending", fmt.Errorf("wrapped: %w", context.Canceled), true, DispositionKeep},
		{"wrapped cancel, not suspending", fmt.Errorf("wrapped: %w", context.Canceled), false, DispositionDelete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := TerminalOutcome(tc.runErr, tc.suspending); got != tc.want {
				t.Fatalf("TerminalOutcome(%v, %v) = %v, want %v", tc.runErr, tc.suspending, got, tc.want)
			}
		})
	}
}

// TestSetting_ReconstructibleFollowsTheSource: a value the run did not set is
// reconstructible (the profile supplies the default); a value the profile
// supplied is reconstructible only when THAT profile is the run's agent; a
// per-run override is never reconstructible.
func TestSetting_ReconstructibleFollowsTheSource(t *testing.T) {
	for _, tc := range []struct {
		name    string
		setting Setting
		agent   string
		want    bool
	}{
		{"unset", Setting{}, "ops", true},
		{"unset, no agent", Setting{}, "", true},
		{"profile supplied, same agent", Setting{Set: true, SourceProfile: "ops"}, "ops", true},
		{"profile supplied, other agent", Setting{Set: true, SourceProfile: "ops"}, "other", false},
		{"per-run override", Setting{Set: true, SourceProfile: ""}, "ops", false},
		{"per-run override, no agent", Setting{Set: true, SourceProfile: ""}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.setting.Reconstructible(tc.agent); got != tc.want {
				t.Fatalf("Reconstructible(%q) = %v, want %v", tc.agent, got, tc.want)
			}
		})
	}
}

// TestJudgeResumability_AProfileRunResumesAndAnOverrideDoesNot: the
// resumability rule. The profile's own defaults are reloaded on boot, so they
// are fine; a per-run override is recorded nowhere the resumer reads.
func TestJudgeResumability_AProfileRunResumesAndAnOverrideDoesNot(t *testing.T) {
	profile := Setting{Set: true, SourceProfile: "ops"}
	for _, tc := range []struct {
		name  string
		facts DispatchFacts
		want  Resumability
	}{
		{"profile defaults", DispatchFacts{AgentSlug: "ops", System: profile, Model: profile}, Resumable},
		{"nothing set at all", DispatchFacts{}, Resumable},
		{"per-run system override", DispatchFacts{AgentSlug: "ops", System: Setting{Set: true}, Model: profile}, NotResumable},
		{"per-run model override", DispatchFacts{AgentSlug: "ops", System: profile, Model: Setting{Set: true}}, NotResumable},
		{"per-run tool allowlist", DispatchFacts{AgentSlug: "ops", System: profile, Model: profile, ToolsOverridden: true}, NotResumable},
		{"system from another profile", DispatchFacts{AgentSlug: "ops", System: Setting{Set: true, SourceProfile: "other"}, Model: profile}, NotResumable},
		{"model from another profile", DispatchFacts{AgentSlug: "ops", System: profile, Model: Setting{Set: true, SourceProfile: "other"}}, NotResumable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := JudgeResumability(tc.facts); got != tc.want {
				t.Fatalf("JudgeResumability = %v, want %v", got, tc.want)
			}
		})
	}
}
