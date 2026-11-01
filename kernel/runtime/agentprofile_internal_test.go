// SPDX-License-Identifier: MIT

package runtime

import (
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/modules/agents/roster"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
)

// The profile BINDING moved to the runs module in W3.76c (runctx.WithAgentProfile
// and the context rows it stamps). The daemon keeps only what is the daemon's:
// the CONFIG OVERRIDE PARSERS that turn the bound map into a run's config, and
// the issue reporting they do.

func TestAgentConfigOverrideParsers(t *testing.T) {
	overrides := map[string]string{
		"AGEZT_MODEL":                    "agent-model",
		"AGEZT_MAX_ITER":                 "7",
		"AGEZT_OBSERVATION_DELTAS":       "enabled",
		"AGEZT_AUTO_CONTINUE_WAIT":       "3s",
		"AGEZT_DISABLE_HEURISTIC_BYPASS": "disabled",
	}
	// A profile carries the overrides via context in production; check the
	// context plumbing hands back exactly what was set.
	ctx := runctx.WithAgentProfile(t.Context(), roster.Profile{Slug: "guarded", ConfigOverrides: overrides})
	var cfg Config
	if issues := applyAgentOverrides(&cfg, runctx.AgentConfigOverrides(ctx)); len(issues) != 0 {
		t.Fatalf("well-formed values reported as issues: %+v", issues)
	}
	if cfg.Model != "agent-model" {
		t.Errorf("Model = %q, want agent-model", cfg.Model)
	}
	if cfg.MaxIter != 7 {
		t.Errorf("MaxIter = %d, want 7", cfg.MaxIter)
	}
	if !cfg.ObservationDeltas {
		t.Error(`ObservationDeltas: "enabled" must parse as true`)
	}
	if cfg.AutoContinueWait != 3*time.Second {
		t.Errorf("AutoContinueWait = %v, want 3s", cfg.AutoContinueWait)
	}
	if cfg.DisableHeuristicBypass {
		t.Error(`DisableHeuristicBypass: "disabled" must parse as false`)
	}
}

func TestAgentRuntimeConfigIssues(t *testing.T) {
	issues := agentRuntimeConfigIssues(map[string]string{
		"AGEZT_MODEL":                    "   ",
		"AGEZT_MAX_ITER":                 "abc",
		"AGEZT_AUTO_CONTINUE_WAIT":       "later",
		"AGEZT_DISABLE_HEURISTIC_BYPASS": "maybe",
		"AGEZT_X_MODE":                   "ignored-generic",
	})
	if len(issues) != 4 {
		t.Fatalf("issues = %+v, want 4", issues)
	}
	if issues[0].Key != "AGEZT_MODEL" || issues[0].Issue == "" {
		t.Fatalf("model issue missing detail: %+v", issues[0])
	}
	if issues[3].Key != "AGEZT_DISABLE_HEURISTIC_BYPASS" {
		t.Fatalf("unexpected issue ordering/detail: %+v", issues)
	}
}
