// SPDX-License-Identifier: MIT

package runctx

import (
	"context"
	"strings"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
)

// WithAgentProfile binds a run to a roster profile (M790): its system prompt
// (soul + standing instructions + tasks), model and ordered fallbacks, tool
// allow/deny lists, trust ceiling, lifecycle, retry and noise policy, config
// overrides, memory scope, workdir, and ledger identity. Every way of running
// an agent goes through it, so no entry point grants an agent more than its
// profile does. The per-run cost ceiling is NOT applied here — callers layer
// it so their own explicit budget wins; callers likewise layer explicit
// per-run model/system overrides on top.
func WithAgentProfile(ctx context.Context, p agentapi.Profile) context.Context {
	noise := EffectiveAgentNoisePolicy(p)
	if p.System {
		ctx = context.WithValue(ctx, ctxKeySystemAgent, true)
	}
	if sys := AgentProfileSystem(p); sys != "" {
		ctx = context.WithValue(ctx, ctxKeySystem, RunStringSetting{Value: sys, ProfileSlug: p.Slug})
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
		ctx = context.WithValue(ctx, ctxKeyAgentToolPolicy, AgentToolPolicy{
			Allow: append([]string(nil), p.ToolAllow...),
			Deny:  append([]string(nil), p.ToolDeny...),
		})
	}
	if noise != (AgentNoisePolicy{}) {
		ctx = context.WithValue(ctx, ctxKeyAgentNoisePolicy, noise)
	}
	if len(p.ConfigOverrides) > 0 {
		ctx = context.WithValue(ctx, ctxKeyAgentConfigOverrides, CloneStringMap(p.ConfigOverrides))
	}
	if ceiling := strings.TrimSpace(p.TrustCeiling); ceiling != "" {
		if lvl, err := edict.ParseTrustLevel(ceiling); err == nil {
			ctx = WithTrustCeiling(ctx, lvl)
		}
	}
	primary := strings.TrimSpace(p.Model)
	if primary != "" {
		ctx = context.WithValue(ctx, ctxKeyModel, RunStringSetting{Value: primary, ProfileSlug: p.Slug})
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
	ctx = memoryapi.WithScope(ctx, scope)
	// The agent's working directory (M792): file/shell tools operate inside
	// this workspace subdirectory. Escape-proofed by the setter.
	ctx = toolapi.WithWorkdir(ctx, p.Workdir)
	// And its identity + daily ceiling for the Governor's ledger (M793).
	return WithAgentIdent(ctx, p.Slug, p.MaxDailyMc)
}

// EffectiveAgentNoisePolicy resolves a profile's noise policy, applying the
// floor a system agent gets: a system agent is always silent on success, never
// writes memory, and never notifies below a warning.
func EffectiveAgentNoisePolicy(p agentapi.Profile) AgentNoisePolicy {
	var out AgentNoisePolicy
	if p.NoisePolicy != nil {
		out.SilentOnSuccess = p.NoisePolicy.SilentOnSuccess
		out.DisableMemoryWrites = p.NoisePolicy.DisableMemoryWrites
		out.MinNotifySeverity = strings.ToLower(strings.TrimSpace(p.NoisePolicy.MinNotifySeverity))
		out.MinNotifyIntervalSec = p.NoisePolicy.MinNotifyIntervalSec
	}
	if out.SilentOnSuccess && NotifySeverityRank(out.MinNotifySeverity) < NotifySeverityRank("warning") {
		out.MinNotifySeverity = "warning"
	}
	if p.System {
		out.SilentOnSuccess = true
		out.DisableMemoryWrites = true
		if NotifySeverityRank(out.MinNotifySeverity) < NotifySeverityRank("warning") {
			out.MinNotifySeverity = "warning"
		}
		if out.MinNotifyIntervalSec < 8*3600 {
			out.MinNotifyIntervalSec = 8 * 3600
		}
	}
	return out
}

// AgentNoisePolicyFromCtx returns the run's noise policy and whether one was set.
func AgentNoisePolicyFromCtx(ctx context.Context) (AgentNoisePolicy, bool) {
	v, ok := ctx.Value(ctxKeyAgentNoisePolicy).(AgentNoisePolicy)
	return v, ok
}

// AppendUniqueString appends a trimmed, non-empty value unless the slice
// already carries it (case-insensitively).
func AppendUniqueString(in []string, value string) []string {
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
	// tools (memory, M851) can read who is acting via toolapi.AgentFromContext —
	// the runtime key here is private and additionally carries the daily ceiling.
	ctx = toolapi.WithAgent(ctx, slug)
	return context.WithValue(ctx, ctxKeyAgentIdent, agentIdent{slug: slug, dailyMc: dailyMc})
}

// AgentIdentFromCtx returns the run's named agent and its daily ceiling.
func AgentIdentFromCtx(ctx context.Context) (string, int64) {
	if v, ok := ctx.Value(ctxKeyAgentIdent).(agentIdent); ok {
		return v.slug, v.dailyMc
	}
	return "", 0
}

// AgentSlugFromCtx returns the run's named agent, or "" when the run has none.
func AgentSlugFromCtx(ctx context.Context) string { s, _ := AgentIdentFromCtx(ctx); return s }

// AgentDailyMcFromCtx returns the run's named agent's daily ceiling.
func AgentDailyMcFromCtx(ctx context.Context) int64 { _, d := AgentIdentFromCtx(ctx); return d }
