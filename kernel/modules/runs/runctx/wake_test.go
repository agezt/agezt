// SPDX-License-Identifier: MIT

package runctx

import (
	"context"
	"testing"
)

// The wake half of the run context: WHY the run exists, WHO acts, and what the
// run may call. The wake provenance and the tool allowlist are CONTRACT rows
// (agentapi, toolapi) — the module's setters are thin delegates over them,
// and these tests pin both halves: the delegate forwards, and the module's own
// keys carry what the contract has no row for.

func TestWithWakeContext_TravelsWithTheRun(t *testing.T) {
	ctx := WithWakeContext(context.Background(), WakeContext{
		Source:            "schedule",
		Reason:            "tick",
		ParentCorrelation: "corr-parent",
	})
	got := WakeContextFromCtx(ctx)
	if got.Source != "schedule" || got.Reason != "tick" {
		t.Fatalf("wake context = %+v, want the stated provenance", got)
	}
	if got.ParentCorrelation != "corr-parent" {
		t.Fatalf("wake context = %+v, want the parent correlation", got)
	}
	if WakeContextFromCtx(context.Background()).Source != "" {
		t.Fatal("a bare context carries no wake provenance")
	}
}

// TestWithTools_CarriesTheAllowlist pins the per-run tool allowlist: a
// non-nil slice activates the restriction (an EMPTY one means "no tools"),
// and an absent allowlist means "no restriction".
func TestWithTools_CarriesTheAllowlist(t *testing.T) {
	if _, ok := ToolsFromCtx(context.Background()); ok {
		t.Fatal("an absent allowlist must mean no restriction")
	}
	ctx := WithTools(context.Background(), []string{"shell"})
	allow, ok := ToolsFromCtx(ctx)
	if !ok || len(allow) != 1 || allow[0] != "shell" {
		t.Fatalf("allowlist = %+v/%v, want [shell]", allow, ok)
	}
	empty, ok := ToolsFromCtx(WithTools(context.Background(), []string{}))
	if !ok || len(empty) != 0 {
		t.Fatalf("allowlist = %+v/%v, want an empty but SET allowlist", empty, ok)
	}
}

// TestWithActor_CarriesTheActorAndTheCorrelation pins the attribution pair:
// the approval path needs both the actor and the correlation of the run that
// asked for it.
func TestWithActor_CarriesTheActorAndTheCorrelation(t *testing.T) {
	if ActorFromCtx(context.Background()) != "" || CorrelationFromCtx(context.Background()) != "" {
		t.Fatal("a bare context carries no actor and no correlation")
	}
	ctx := WithActor(context.Background(), "operator", "corr-1")
	if got := ActorFromCtx(ctx); got != "operator" {
		t.Fatalf("actor = %q, want operator", got)
	}
	if got := CorrelationFromCtx(ctx); got != "corr-1" {
		t.Fatalf("correlation = %q, want corr-1", got)
	}
}

// TestWithModelChain_CarriesTheOrderedFallbacks pins the ordered fallback
// chain: an empty chain is a no-op (the run keeps the task type's chain).
func TestWithModelChain_CarriesTheOrderedFallbacks(t *testing.T) {
	if got := ModelChainFromCtx(context.Background()); got != nil {
		t.Fatalf("model chain = %+v, want none", got)
	}
	if got := ModelChainFromCtx(WithModelChain(context.Background(), nil)); got != nil {
		t.Fatalf("model chain = %+v, want none", got)
	}
	ctx := WithModelChain(context.Background(), []string{"m-1", "m-2"})
	got := ModelChainFromCtx(ctx)
	if len(got) != 2 || got[0] != "m-1" || got[1] != "m-2" {
		t.Fatalf("model chain = %+v, want [m-1 m-2]", got)
	}
}

// TestAgentRetryPolicyFromCtx_ReportsAbsence pins the "was it set?" half of
// the retry policy read: an unset policy is reported as absent, not as an
// empty policy that would look like "retry never".
func TestAgentRetryPolicyFromCtx_ReportsAbsence(t *testing.T) {
	if _, ok := AgentRetryPolicyFromCtx(context.Background()); ok {
		t.Fatal("an absent retry policy must be reported as absent")
	}
}
