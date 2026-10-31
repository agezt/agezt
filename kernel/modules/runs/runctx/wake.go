// SPDX-License-Identifier: MIT

package runctx

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// WithWakeContext attaches run provenance to the next agent loop. Empty fields
// are omitted from the journal; callers can layer it with WithAgentProfile.
//
// The row and the context key live in the CONTRACT layer (agentapi): a host
// that starts a run states WHY the run exists, and it must not have to name
// this module to do it.
func WithWakeContext(ctx context.Context, w WakeContext) context.Context {
	return agentapi.WithWakeContext(ctx, w)
}

// WakeContextFromCtx returns the run's wake provenance.
func WakeContextFromCtx(ctx context.Context) WakeContext {
	v, _ := agentapi.WakeContextFrom(ctx)
	return v
}

// WithActor stamps the actor and the correlation the run runs under, so the
// approval path can attribute the audit events to the originating task.
func WithActor(ctx context.Context, actor, corr string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyActor, actor)
	return context.WithValue(ctx, ctxKeyCorrelation, corr)
}

// ActorFromCtx returns the run's actor.
func ActorFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyActor).(string); ok {
		return v
	}
	return ""
}

// CorrelationFromCtx returns the run's correlation.
func CorrelationFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyCorrelation).(string); ok {
		return v
	}
	return ""
}

// SystemAgentFromCtx reports whether the run is a SYSTEM agent.
func SystemAgentFromCtx(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeySystemAgent).(bool)
	return v
}

// AgentToolPolicyFromCtx returns the run's named agent's tool allow/deny list.
func AgentToolPolicyFromCtx(ctx context.Context) AgentToolPolicy {
	v, _ := ctx.Value(ctxKeyAgentToolPolicy).(AgentToolPolicy)
	return v
}

// AgentRetryPolicyFromCtx returns the run's named agent's retry policy and
// whether one was set.
func AgentRetryPolicyFromCtx(ctx context.Context) (agentapi.RetryPolicy, bool) {
	v, ok := ctx.Value(ctxKeyAgentRetryPolicy).(agentapi.RetryPolicy)
	if !ok {
		return agentapi.RetryPolicy{}, false
	}
	v.RetryOn = append([]string(nil), v.RetryOn...)
	return v, true
}

// AgentConfigOverrides returns the named agent's config-override map attached to
// a run context by WithAgentProfile. The returned map is a copy and safe for the
// caller to mutate. Nil means the run carries no agent-specific overrides.
func AgentConfigOverrides(ctx context.Context) map[string]string {
	v, _ := ctx.Value(ctxKeyAgentConfigOverrides).(map[string]string)
	return CloneStringMap(v)
}

// WithModelChain sets the run's per-agent ordered model fallback chain (M787):
// the Governor tries these models in order, overriding the task type's
// configured chain. Carries a named agent's own fallbacks (roster M783).
func WithModelChain(ctx context.Context, chain []string) context.Context {
	if len(chain) == 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyModelChain, chain)
}

// ModelChainFromCtx returns the run's ordered model fallback chain.
func ModelChainFromCtx(ctx context.Context) []string {
	if v, ok := ctx.Value(ctxKeyModelChain).([]string); ok {
		return v
	}
	return nil
}

// WithTools restricts the run started with this context to the named tools only
// (a per-run allowlist). A non-nil slice — including an EMPTY one (no tools at all,
// for a pure-reasoning / safe one-off run) — activates the restriction; passing it
// is the only way to override, so an unrestricted run is simply one where this is
// never called. Names not registered are ignored.
//
// The allowlist and its context key live in the CONTRACT layer (toolapi): the
// application decides which tools a run may call, and a host that starts the
// run states that decision without naming this module.
func WithTools(ctx context.Context, allow []string) context.Context {
	return toolapi.WithTools(ctx, allow)
}

// ToolsFromCtx returns the per-run tool allowlist and whether one was set. ok=false
// means "no restriction" (use all tools); ok=true with an empty/nil slice means
// "no tools".
func ToolsFromCtx(ctx context.Context) ([]string, bool) {
	return toolapi.ToolsFrom(ctx)
}
