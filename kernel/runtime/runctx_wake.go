// SPDX-License-Identifier: MIT

// Run context wake: WithWakeContext + wakeContextFromCtx + systemAgentFromCtx + agentToolPolicyFromCtx + agentRetryPolicyFromCtx + AgentConfigOverrides + WithModelChain + modelChainFromCtx + WithTools + toolsFromCtx + actorFromCtx + correlationFromCtx.
// Code extracted from runctx.go during the Day-69 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"github.com/agezt/agezt/kernel/roster"
	"strings"
)


// WithWakeContext attaches run provenance to the next agent loop. Empty fields
// are omitted from the journal; callers can layer it with WithAgentProfile.
func WithWakeContext(ctx context.Context, w WakeContext) context.Context {
	w.Source = strings.TrimSpace(w.Source)
	w.Reason = strings.TrimSpace(w.Reason)
	w.ScheduleID = strings.TrimSpace(w.ScheduleID)
	w.StandingID = strings.TrimSpace(w.StandingID)
	w.StandingName = strings.TrimSpace(w.StandingName)
	w.TriggerSubject = strings.TrimSpace(w.TriggerSubject)
	w.ParentCorrelation = strings.TrimSpace(w.ParentCorrelation)
	if w == (WakeContext{}) {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyWakeContext, w)
}

func wakeContextFromCtx(ctx context.Context) WakeContext {
	v, _ := ctx.Value(ctxKeyWakeContext).(WakeContext)
	return v
}

func systemAgentFromCtx(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeySystemAgent).(bool)
	return v
}

func agentToolPolicyFromCtx(ctx context.Context) agentToolPolicy {
	v, _ := ctx.Value(ctxKeyAgentToolPolicy).(agentToolPolicy)
	return v
}

func agentRetryPolicyFromCtx(ctx context.Context) (roster.RetryPolicy, bool) {
	v, ok := ctx.Value(ctxKeyAgentRetryPolicy).(roster.RetryPolicy)
	if !ok {
		return roster.RetryPolicy{}, false
	}
	v.RetryOn = append([]string(nil), v.RetryOn...)
	return v, true
}

// AgentConfigOverrides returns the named agent's config-override map attached to
// a run context by WithAgentProfile. The returned map is a copy and safe for the
// caller to mutate. Nil means the run carries no agent-specific overrides.
func AgentConfigOverrides(ctx context.Context) map[string]string {
	v, _ := ctx.Value(ctxKeyAgentConfigOverrides).(map[string]string)
	return cloneStringMap(v)
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

func modelChainFromCtx(ctx context.Context) []string {
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
func WithTools(ctx context.Context, allow []string) context.Context {
	return context.WithValue(ctx, ctxKeyTools, allow)
}

// toolsFromCtx returns the per-run tool allowlist and whether one was set. ok=false
// means "no restriction" (use all tools); ok=true with an empty/nil slice means
// "no tools".
func toolsFromCtx(ctx context.Context) ([]string, bool) {
	v, ok := ctx.Value(ctxKeyTools).([]string)
	return v, ok
}

func actorFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyActor).(string); ok {
		return v
	}
	return ""
}

func correlationFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyCorrelation).(string); ok {
		return v
	}
	return ""
}
