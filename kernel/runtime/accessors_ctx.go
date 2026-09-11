// SPDX-License-Identifier: MIT

// Kernel accessors: Reflect + StartTime + Plugins + memory/skill flags + ctx helpers (WakeContext* + Images/JSONMode/MaxCost/RunTimeout/Resume* + BuildRunPrompt/InjectHostEnvironment/ResumeCheckpointFn/ResolveRunModel/MergeAutoApproveCapabilities/WithActorCorrelation/ActorFromCtx).
// Code extracted from accessors.go during the Day-56 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/reflect"
	"github.com/agezt/agezt/kernel/skill"
)


// Reflect returns the reflection engine backing `agt reflect` and the optional
// periodic reflection trigger. Always non-nil after Open.
func (k *Kernel) Reflect() *reflect.Engine { return k.reflect }

// StartTime returns the wall-clock time Open() returned. Used by
// `agt status` to compute uptime; not adjusted by Reload or any
// in-process state change, so it reflects "since this process
// started" rather than "since the kernel was last reconfigured".
func (k *Kernel) StartTime() time.Time { return k.startTime }

// Plugins returns the external-plugin manifest the daemon
// supplied at Open(). Read-only — callers must not mutate the
// slice. Used by the control plane to power `agt plugin list`;
// returns nil when no external plugins are configured.
func (k *Kernel) Plugins() []PluginInfo { return k.cfg.Plugins }

// MemoryDistill reports whether post-run memory distillation is
// enabled. The run engine reads this via KernelAPI to decide
// whether to spend one best-effort LLM call folding the journal
// after a multi-tool run (see MaybeDistill).
func (k *Kernel) MemoryDistill() bool { return k.cfg.MemoryDistill }

// MemoryDistillMinTools returns the tool-call threshold that
// triggers a post-run distillation. 0 / unset = the run engine
// uses its own default of 4. Wraps the config field so the runexec
// sub-package can read it through KernelAPI (Day 32).
func (k *Kernel) MemoryDistillMinTools() int { return k.cfg.MemoryDistillMinTools }

// SkillForge reports whether post-run skill-forging is enabled.
// The run engine reads this via KernelAPI to decide whether to
// spend one best-effort LLM call proposing a DRAFT skill from
// the journal after a multi-tool run (see MaybeForge).
func (k *Kernel) SkillForge() bool { return k.cfg.SkillForge }

// SkillForgeMinTools returns the tool-call threshold that
// triggers a post-run skill proposal. 0 / unset = the run engine
// uses its own default of 4. Wraps the config field so the
// runexec sub-package can read it through KernelAPI (Day 32).
func (k *Kernel) SkillForgeMinTools() int { return k.cfg.SkillForgeMinTools }

// SystemAgentFromCtx reports whether the current run is operating
// in system-agent mode (bypasses post-run distillation / forge /
// shadow-eval). Wraps the package-level systemAgentFromCtx so
// the runexec sub-package can read it through KernelAPI (Day 34).
func (k *Kernel) SystemAgentFromCtx(ctx context.Context) bool { return systemAgentFromCtx(ctx) }

// AgentDailyMcFromCtx returns the named agent's daily USD-microcent
// ceiling attached to a run context by WithAgentProfile. Wraps the
// package-level agentDailyMcFromCtx (Day 34).
func (k *Kernel) AgentDailyMcFromCtx(ctx context.Context) int64 { return agentDailyMcFromCtx(ctx) }

// ModelChainFromCtx returns the explicit per-request model chain
// attached to a run context by WithModelChain. Wraps the
// package-level modelChainFromCtx (Day 34).
func (k *Kernel) ModelChainFromCtx(ctx context.Context) []string { return modelChainFromCtx(ctx) }

// WakeContextSource returns the run provenance "source" field
// (e.g. "schedule", "channel", "standing"). Wraps the
// package-level wakeContextFromCtx so the runexec sub-package
// can read individual fields through KernelAPI (Day 34) without
// importing the WakeContext type across the package boundary.
func (k *Kernel) WakeContextSource(ctx context.Context) string { return wakeContextFromCtx(ctx).Source }

// WakeContextReason returns the run provenance "reason" field
// (e.g. "tick", "command", "briefing"). Wraps wakeContextFromCtx.
func (k *Kernel) WakeContextReason(ctx context.Context) string { return wakeContextFromCtx(ctx).Reason }

// WakeContextScheduleID returns the schedule ID that triggered
// the run ("" when the run did not originate from a schedule).
// Wraps wakeContextFromCtx.
func (k *Kernel) WakeContextScheduleID(ctx context.Context) string { return wakeContextFromCtx(ctx).ScheduleID }

// WakeContextStandingID returns the standing order ID that
// triggered the run. Wraps wakeContextFromCtx.
func (k *Kernel) WakeContextStandingID(ctx context.Context) string { return wakeContextFromCtx(ctx).StandingID }

// WakeContextStandingName returns the standing order name that
// triggered the run. Wraps wakeContextFromCtx.
func (k *Kernel) WakeContextStandingName(ctx context.Context) string { return wakeContextFromCtx(ctx).StandingName }

// WakeContextTriggerSubject returns the bus subject that
// triggered the run (e.g. "channel.telegram.message"). Wraps
// wakeContextFromCtx.
func (k *Kernel) WakeContextTriggerSubject(ctx context.Context) string { return wakeContextFromCtx(ctx).TriggerSubject }

// WakeContextParentCorrelation returns the parent run's
// correlation ID (the lead run that delegated this child). Wraps
// wakeContextFromCtx.
func (k *Kernel) WakeContextParentCorrelation(ctx context.Context) string { return wakeContextFromCtx(ctx).ParentCorrelation }

// ImagesFromCtx returns the images attached to a run context by
// WithImages (vision-gated upstream by M91). Wraps the
// package-level imagesFromCtx (Day 34).
func (k *Kernel) ImagesFromCtx(ctx context.Context) []string { return imagesFromCtx(ctx) }

// JSONModeFromCtx reports whether the current run was requested
// with structured-output (response_format=json_object by OpenAI,
// tool_choice="any" by Anthropic). Wraps the package-level
// jsonModeFromCtx (Day 34).
func (k *Kernel) JSONModeFromCtx(ctx context.Context) bool { return jsonModeFromCtx(ctx) }

// MaxCostFromCtx returns the per-run USD-microcent ceiling attached
// to a run context by WithMaxCost (--max-cost). Wraps the
// package-level maxCostFromCtx (Day 34).
func (k *Kernel) MaxCostFromCtx(ctx context.Context) int64 { return maxCostFromCtx(ctx) }

// RunTimeoutFromCtx returns the per-run timeout attached by
// WithRunTimeout (`agt run --timeout`). Wraps the package-level
// runTimeoutFromCtx (Day 34).
func (k *Kernel) RunTimeoutFromCtx(ctx context.Context) time.Duration { return runTimeoutFromCtx(ctx) }

// ResumeOwnedKindFromCtx returns (kind, true) if the run context
// carries a resume-ticket owned by an outer wrapper (assure /
// retry / resumer). Wraps the package-level resumeOwnedKind (Day 34).
func (k *Kernel) ResumeOwnedKindFromCtx(ctx context.Context) (string, bool) { return resumeOwnedKind(ctx) }

// ResumeSeedFromCtx returns (prior messages, start iter, true) if
// the run context carries a seeded conversation snapshot (resumed
// run). Wraps the package-level resumeSeedFromCtx (Day 34).
func (k *Kernel) ResumeSeedFromCtx(ctx context.Context) ([]agent.Message, int, bool) {
	return resumeSeedFromCtx(ctx)
}

// DisableHeuristicBypass reports whether THIS run's effective
// config disables the deterministic heuristic bypass (time/date
// fast paths). Wraps the effectiveConfig field the Runner needs
// without leaking the full Config type across the package
// boundary (Day 34).
func (k *Kernel) DisableHeuristicBypass(ctx context.Context) bool {
	return k.effectiveConfig(ctx).DisableHeuristicBypass
}

// BuildRunPrompt assembles the system prompt (live default +
// profile/taste/memory/world/skill injection) for a run. Day 34
// wrapper for the runexec sub-package.
func (k *Kernel) BuildRunPrompt(runCtx context.Context, corr, actor, intent string, systemAgent bool, skillDirective skill.ActivationDirective) (string, []string) {
	return k.buildRunPrompt(runCtx, corr, actor, intent, systemAgent, skillDirective)
}

// InjectHostEnvironment prepends the host-environment preamble to
// the system prompt when Config.EnvironmentInject is set. Day 34
// wrapper for the runexec sub-package.
func (k *Kernel) InjectHostEnvironment(system string, tools map[string]agent.Tool) string {
	return k.injectHostEnvironment(system, tools)
}

// ResumeCheckpointFn returns the per-iteration checkpoint callback
// for a resumed run (M1002). Wraps the package-level resume.go
// helper so the runexec sub-package can drive it through KernelAPI.
func (k *Kernel) ResumeCheckpointFn(corr string) func(int, []agent.Message) {
	return k.resumeCheckpointFn(corr)
}

// ResolveRunModel picks the per-request model from the run ctx
// (explicit > daemon default with agent-profile overrides applied).
// Runs k.effectiveConfig internally so live daemon-default swaps
// (M816) + per-agent overrides (M783) are reflected without the
// Runner needing the Config type across the package boundary
// (Day 34).
func (k *Kernel) ResolveRunModel(ctx context.Context) (string, bool) {
	if m := modelFromCtx(ctx); m != "" {
		return m, true
	}
	return k.effectiveConfig(ctx).Model, false
}

// MergeAutoApproveCapabilities merges a per-run auto-approve list
// with the existing ctx-stashed one. Wraps the package-level helper.
func (k *Kernel) MergeAutoApproveCapabilities(ctx context.Context, from map[string]bool) map[string]bool {
	return mergeAutoApproveCapabilities(ctx, from)
}

// WithActorCorrelation stamps the actor + correlation IDs onto a
// context for downstream consumers (agent.PolicyHook, the
// in-process memory tool). Wraps the ctxKeyActor + ctxKeyCorrelation
// writes so the runexec sub-package can decorate a runCtx
// without importing the runtime ctx key types (Day 34).
func (k *Kernel) WithActorCorrelation(ctx context.Context, actor, corr string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyActor, actor)
	ctx = context.WithValue(ctx, ctxKeyCorrelation, corr)
	return ctx
}

// ActorFromCtx returns the actor name stamped by WithActorCorrelation.
func (k *Kernel) ActorFromCtx(ctx context.Context) string {
	s, _ := ctx.Value(ctxKeyActor).(string)
	return s
}

// ShadowEval reports whether post-run shadow-skill judgement is
// enabled (SPEC-05 §5.2). When true, the run engine evaluates
// the shadow skills relevant to a completed run against what
// actually happened — opt-in, best-effort, no side effects (see
// MaybeShadowEval).
func (k *Kernel) ShadowEval() bool { return k.cfg.ShadowEval }