// SPDX-License-Identifier: MIT

// Run context agent: WithAgentProfile + effectiveAgentNoisePolicy + agentNoisePolicyFromCtx + appendUniqueString + WithAgentIdent + agentIdentFromCtx + agentSlugFromCtx + agentDailyMcFromCtx.
// Code extracted from runctx.go during the Day-69 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/roster"
	"strings"
)


// application (its model resolves before the vision gate).
func WithAgentProfile(ctx context.Context, p roster.Profile) context.Context {
	noise := effectiveAgentNoisePolicy(p)
	if p.System {
		ctx = context.WithValue(ctx, ctxKeySystemAgent, true)
	}
	if sys := agentProfileSystem(p); sys != "" {
		ctx = WithSystem(ctx, sys)
	}
	if p.Lifecycle.Mode != "" || p.Lifecycle.RetireOnComplete {
		ctx = context.WithValue(ctx, ctxKeyAgentLifecycle, p.Lifecycle)
	}
	if p.RetryPolicy != nil {
		cp := *p.RetryPolicy
		cp.RetryOn = append([]string(nil), p.RetryPolicy.RetryOn...)
		ctx = context.WithValue(ctx, ctxKeyAgentRetryPolicy, cp)
	}
	if len(p.ToolAllow) > 0 || len(p.ToolDeny) > 0 {
		ctx = context.WithValue(ctx, ctxKeyAgentToolPolicy, agentToolPolicy{
			allow: append([]string(nil), p.ToolAllow...),
			deny:  append([]string(nil), p.ToolDeny...),
		})
	}
	if noise != (agentNoisePolicy{}) {
		ctx = context.WithValue(ctx, ctxKeyAgentNoisePolicy, noise)
	}
	if len(p.ConfigOverrides) > 0 {
		ctx = context.WithValue(ctx, ctxKeyAgentConfigOverrides, cloneStringMap(p.ConfigOverrides))
	}
	if ceiling := strings.TrimSpace(p.TrustCeiling); ceiling != "" {
		if lvl, err := edict.ParseTrustLevel(ceiling); err == nil {
			ctx = WithTrustCeiling(ctx, lvl)
		}
	}
	primary := strings.TrimSpace(p.Model)
	if primary != "" {
		ctx = WithModel(ctx, primary)
	}
	if len(p.Fallbacks) > 0 {
		chain := []string{primary}
		if primary == "" {
			chain = nil
		}
		for _, m := range p.Fallbacks {
			if m = strings.TrimSpace(m); m != "" && m != primary {
				chain = append(chain, m)
			}
		}
		ctx = WithModelChain(ctx, chain)
	}
	scope := strings.TrimSpace(p.MemoryScope)
	if scope == "" {
		scope = p.Slug
	}
	ctx = memory.WithScope(ctx, scope)
	// The agent's working directory (M792): file/shell tools operate inside
	// this workspace subdirectory. Escape-proofed by the setter.
	ctx = agent.WithWorkdir(ctx, p.Workdir)
	// And its identity + daily ceiling for the Governor's ledger (M793).
	return WithAgentIdent(ctx, p.Slug, p.MaxDailyMc)
}

func effectiveAgentNoisePolicy(p roster.Profile) agentNoisePolicy {
	var out agentNoisePolicy
	if p.NoisePolicy != nil {
		out.silentOnSuccess = p.NoisePolicy.SilentOnSuccess
		out.disableMemoryWrites = p.NoisePolicy.DisableMemoryWrites
		out.minNotifySeverity = strings.ToLower(strings.TrimSpace(p.NoisePolicy.MinNotifySeverity))
		out.minNotifyIntervalSec = p.NoisePolicy.MinNotifyIntervalSec
	}
	if out.silentOnSuccess && notifySeverityRank(out.minNotifySeverity) < notifySeverityRank("warning") {
		out.minNotifySeverity = "warning"
	}
	if p.System {
		out.silentOnSuccess = true
		out.disableMemoryWrites = true
		if notifySeverityRank(out.minNotifySeverity) < notifySeverityRank("warning") {
			out.minNotifySeverity = "warning"
		}
		if out.minNotifyIntervalSec < 8*3600 {
			out.minNotifyIntervalSec = 8 * 3600
		}
	}
	return out
}

func agentNoisePolicyFromCtx(ctx context.Context) (agentNoisePolicy, bool) {
	v, ok := ctx.Value(ctxKeyAgentNoisePolicy).(agentNoisePolicy)
	return v, ok
}

func appendUniqueString(in []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return in
	}
	for _, x := range in {
		if strings.EqualFold(strings.TrimSpace(x), value) {
			return in
		}
	}
	return append(in, value)
}

// WithAgentIdent stamps the run with a named agent's identity and per-day
// spend ceiling (M793): every completion of the run is metered against the
// Governor's per-agent daily ledger and refused past the ceiling.
func WithAgentIdent(ctx context.Context, slug string, dailyMc int64) context.Context {
	if strings.TrimSpace(slug) == "" {
		return ctx
	}
	// Also stamp the agent slug under the kernel/agent key so provenance-aware
	// tools (memory, M851) can read who is acting via agent.AgentFromContext —
	// the runtime key here is private and additionally carries the daily ceiling.
	ctx = agent.WithAgent(ctx, slug)
	return context.WithValue(ctx, ctxKeyAgentIdent, agentIdent{slug: slug, dailyMc: dailyMc})
}

func agentIdentFromCtx(ctx context.Context) (string, int64) {
	if v, ok := ctx.Value(ctxKeyAgentIdent).(agentIdent); ok {
		return v.slug, v.dailyMc
	}
	return "", 0
}

func agentSlugFromCtx(ctx context.Context) string { s, _ := agentIdentFromCtx(ctx); return s }

func agentDailyMcFromCtx(ctx context.Context) int64 { _, d := agentIdentFromCtx(ctx); return d }
