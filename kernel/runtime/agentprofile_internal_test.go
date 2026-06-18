// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/roster"
)

func TestWithAgentProfile_TrustCeilingAppliesToContext(t *testing.T) {
	ctx := WithAgentProfile(context.Background(), roster.Profile{
		Slug:         "guarded",
		TrustCeiling: "L2",
	})
	lvl, ok := trustCeilingFromCtx(ctx)
	if !ok || lvl != edict.LevelAskFirst {
		t.Fatalf("trust ceiling from profile = %v/%v, want L2", lvl, ok)
	}
}

func TestWithAgentProfile_ConfigOverridesApplyToContext(t *testing.T) {
	ctx := WithAgentProfile(context.Background(), roster.Profile{
		Slug: "guarded",
		ConfigOverrides: map[string]string{
			"AGEZT_X_MODE": "agent",
		},
	})
	got := AgentConfigOverrides(ctx)
	if got["AGEZT_X_MODE"] != "agent" {
		t.Fatalf("config overrides from profile = %+v", got)
	}
	got["AGEZT_X_MODE"] = "mutated"
	if AgentConfigOverrides(ctx)["AGEZT_X_MODE"] != "agent" {
		t.Fatal("AgentConfigOverrides should return a defensive copy")
	}
}

func TestWithAgentProfile_RetryPolicyAppliesToContext(t *testing.T) {
	ctx := WithAgentProfile(context.Background(), roster.Profile{
		Slug: "guarded",
		RetryPolicy: &roster.RetryPolicy{
			MaxAttempts: 3,
			Backoff:     "exponential",
			RetryOn:     []string{"error", "timeout"},
		},
	})
	got, ok := agentRetryPolicyFromCtx(ctx)
	if !ok || got.MaxAttempts != 3 || got.Backoff != "exponential" || len(got.RetryOn) != 2 {
		t.Fatalf("retry policy from profile = %+v/%v", got, ok)
	}
	got.RetryOn[0] = "mutated"
	again, _ := agentRetryPolicyFromCtx(ctx)
	if again.RetryOn[0] != "error" {
		t.Fatal("agentRetryPolicyFromCtx should return a defensive copy")
	}
}

func TestAgentConfigOverrideParsers(t *testing.T) {
	ctx := WithAgentProfile(context.Background(), roster.Profile{
		Slug: "guarded",
		ConfigOverrides: map[string]string{
			"AGEZT_MODEL":                    "agent-model",
			"AGEZT_MAX_ITER":                 "7",
			"AGEZT_OBSERVATION_DELTAS":       "enabled",
			"AGEZT_AUTO_CONTINUE_WAIT":       "3s",
			"AGEZT_DISABLE_HEURISTIC_BYPASS": "disabled",
		},
	})
	if got, ok := agentConfigStringOverride(ctx, "AGEZT_MODEL"); !ok || got != "agent-model" {
		t.Fatalf("string override = %q/%v", got, ok)
	}
	if got, ok := agentConfigIntOverride(ctx, "AGEZT_MAX_ITER"); !ok || got != 7 {
		t.Fatalf("int override = %d/%v", got, ok)
	}
	if got, ok := agentConfigBoolOverride(ctx, "AGEZT_OBSERVATION_DELTAS"); !ok || !got {
		t.Fatalf("bool override = %v/%v", got, ok)
	}
	if got, ok := agentConfigDurationOverride(ctx, "AGEZT_AUTO_CONTINUE_WAIT"); !ok || got != 3*time.Second {
		t.Fatalf("duration override = %v/%v", got, ok)
	}
	if got, ok := agentConfigBoolOverride(ctx, "AGEZT_DISABLE_HEURISTIC_BYPASS"); !ok || got {
		t.Fatalf("bool off override = %v/%v", got, ok)
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
