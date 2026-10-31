// SPDX-License-Identifier: MIT

package runctx

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/edict"
)

// The profile binding is the run's dispatch decision: which identity, which
// model, which tools, which ceilings. It moved to the runs module in W3.76c;
// these tests moved with it.

func TestWithAgentProfile_TrustCeilingAppliesToContext(t *testing.T) {
	ctx := WithAgentProfile(context.Background(), agentapi.Profile{
		Slug:         "guarded",
		TrustCeiling: "L2",
	})
	lvl, ok := TrustCeilingFromCtx(ctx)
	if !ok || lvl != edict.LevelAskFirst {
		t.Fatalf("trust ceiling from profile = %v/%v, want L2", lvl, ok)
	}
}

func TestWithAgentProfile_ConfigOverridesApplyToContext(t *testing.T) {
	ctx := WithAgentProfile(context.Background(), agentapi.Profile{
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
	ctx := WithAgentProfile(context.Background(), agentapi.Profile{
		Slug: "guarded",
		RetryPolicy: &agentapi.RetryPolicy{
			MaxAttempts: 3,
			Backoff:     "exponential",
			RetryOn:     []string{"error", "timeout"},
		},
	})
	got, ok := AgentRetryPolicyFromCtx(ctx)
	if !ok || got.MaxAttempts != 3 || got.Backoff != "exponential" || len(got.RetryOn) != 2 {
		t.Fatalf("retry policy from profile = %+v/%v", got, ok)
	}
	got.RetryOn[0] = "mutated"
	again, _ := AgentRetryPolicyFromCtx(ctx)
	if again.RetryOn[0] != "error" {
		t.Fatal("AgentRetryPolicyFromCtx should return a defensive copy")
	}
}

// TestWithAgentProfile_CopiesTheRetryPolicy pins the COPY: the caller keeps
// ownership of its own profile, so mutating the profile after the binding must
// not change the run that is already under way.
func TestWithAgentProfile_CopiesTheRetryPolicy(t *testing.T) {
	policy := &agentapi.RetryPolicy{MaxAttempts: 3, RetryOn: []string{"error"}}
	ctx := WithAgentProfile(context.Background(), agentapi.Profile{
		Slug:        "guarded",
		RetryPolicy: policy,
	})
	policy.RetryOn[0] = "mutated"

	got, ok := AgentRetryPolicyFromCtx(ctx)
	if !ok || got.RetryOn[0] != "error" {
		t.Fatalf("the bound retry policy was not copied: %+v", got.RetryOn)
	}
}

// TestWithAgentProfile_StampsTheRunsIdentity pins the identity half of the
// binding: the slug and the daily ceiling ride on the context so the
// Governor's per-agent ledger can meter the run.
func TestWithAgentProfile_StampsTheRunsIdentity(t *testing.T) {
	ctx := WithAgentProfile(context.Background(), agentapi.Profile{
		Slug:       "ops",
		MaxDailyMc: 4321,
	})
	if got := AgentSlugFromCtx(ctx); got != "ops" {
		t.Fatalf("agent slug = %q, want ops", got)
	}
	if got := AgentDailyMcFromCtx(ctx); got != 4321 {
		t.Fatalf("agent daily ceiling = %d, want 4321", got)
	}
}

// TestWithAgentProfile_CarriesTheToolPolicy pins the tool half of the binding:
// the profile's allow/deny lists travel as the run's tool policy.
func TestWithAgentProfile_CarriesTheToolPolicy(t *testing.T) {
	ctx := WithAgentProfile(context.Background(), agentapi.Profile{
		Slug:      "ops",
		ToolAllow: []string{"shell"},
		ToolDeny:  []string{"forge"},
	})
	pol := AgentToolPolicyFromCtx(ctx)
	if len(pol.Allow) != 1 || pol.Allow[0] != "shell" {
		t.Fatalf("tool allow = %+v, want [shell]", pol.Allow)
	}
	if len(pol.Deny) != 1 || pol.Deny[0] != "forge" {
		t.Fatalf("tool deny = %+v, want [forge]", pol.Deny)
	}
}

// TestWithAgentProfile_BindsTheModelAndItsFallbacks pins the model half: the
// primary model becomes the run's model setting (with the profile as its
// source) and the fallbacks become the ordered chain.
func TestWithAgentProfile_BindsTheModelAndItsFallbacks(t *testing.T) {
	ctx := WithAgentProfile(context.Background(), agentapi.Profile{
		Slug:      "ops",
		Model:     "ops-model",
		Fallbacks: []string{"ops-model", "backup-model"},
	})
	setting := ModelSetting(ctx)
	if setting.Value != "ops-model" || setting.ProfileSlug != "ops" {
		t.Fatalf("model setting = %+v, want ops-model from ops", setting)
	}
	chain := ModelChainFromCtx(ctx)
	if len(chain) != 2 || chain[0] != "ops-model" || chain[1] != "backup-model" {
		t.Fatalf("model chain = %+v, want [ops-model backup-model]", chain)
	}
}

// TestWithAgentProfile_BindsTheSoulAsTheSystemPrompt pins the identity half:
// the profile's soul IS the run's system prompt, and it is recorded as
// PROFILE-SOURCED so the boot resumer can reconstruct it.
func TestWithAgentProfile_BindsTheSoulAsTheSystemPrompt(t *testing.T) {
	ctx := WithAgentProfile(context.Background(), agentapi.Profile{
		Slug: "ops",
		Soul: "ops soul",
	})
	setting := SystemSetting(ctx)
	if setting.Value != "ops soul" {
		t.Fatalf("system setting = %+v, want the profile's soul", setting)
	}
	if setting.ProfileSlug != "ops" {
		t.Fatalf("system setting source = %q, want the profile slug", setting.ProfileSlug)
	}
}

// TestWithAgentProfile_SystemAgentIsMarked pins the system-agent flag: a
// system agent skips the post-run learning layers, and the run has to say so.
func TestWithAgentProfile_SystemAgentIsMarked(t *testing.T) {
	if !SystemAgentFromCtx(WithAgentProfile(context.Background(), agentapi.Profile{Slug: "sys", System: true})) {
		t.Fatal("a system profile must mark the run as a system agent")
	}
	if SystemAgentFromCtx(WithAgentProfile(context.Background(), agentapi.Profile{Slug: "plain"})) {
		t.Fatal("an ordinary profile must not mark the run as a system agent")
	}
}

// TestAppendUniqueString_KeepsTheListUnique pins the small helper the tool
// policy and the noise policy use to add a name: a trimmed, non-empty value
// is appended once, case-insensitively.
func TestAppendUniqueString_KeepsTheListUnique(t *testing.T) {
	got := AppendUniqueString([]string{"shell"}, "  Forge ")
	if len(got) != 2 || got[1] != "Forge" {
		t.Fatalf("append = %+v, want [shell Forge]", got)
	}
	got = AppendUniqueString(got, "SHELL")
	if len(got) != 2 {
		t.Fatalf("append = %+v, want the duplicate to be dropped", got)
	}
	got = AppendUniqueString(got, "   ")
	if len(got) != 2 {
		t.Fatalf("append = %+v, want the blank to be dropped", got)
	}
}

// TestWithAgentProfile_HonoursThePerRunBudgetLayer pins the precedence the
// doc comment claims: the profile does NOT apply the cost ceiling — the
// caller layers it, so an explicit per-run budget wins.
func TestWithAgentProfile_HonoursThePerRunBudgetLayer(t *testing.T) {
	ctx := WithAgentProfile(context.Background(), agentapi.Profile{Slug: "ops", MaxCostMc: 100})
	if got := MaxCostFromCtx(ctx); got != 0 {
		t.Fatalf("profile cost ceiling leaked into the run: %d", got)
	}
	ctx = WithMaxCost(ctx, 100)
	if got := MaxCostFromCtx(ctx); got != 100 {
		t.Fatalf("per-run cost ceiling = %d, want 100", got)
	}
}
