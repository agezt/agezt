// SPDX-License-Identifier: MIT

package runtime

// Provenance: Kernel accessors: Reflect + StartTime + Plugins + memory/skill flags +
//             ctx helpers (Resume* +
//             ResumeCheckpointFn/ResolveRunModel/WithAgentProfile/WithMaxCost).
//             The prompt doors (BuildRunPrompt/InjectHostEnvironment) moved to the
//             runs module's prompt service in W3.76e, and they are GONE since
//             W3.76af: the run engine holds the module's service and asks the
//             module for its own prompt.
//             The RunTimeout and MergeAutoApproveCapabilities doors went with
//             W3.76k: the run's callers read the run context through the module
//             (runctx.RunTimeoutFromCtx / runctx.MergeAutoApproveCapabilities)
//             and the daemon's forwarding copies had no caller left.
//             The run context's READ vocabulary went with W3.76l: fifteen doors
//             (the wake fields, the images, the json mode, the cost ceiling,
//             the model chain, the agent's daily ceiling, the actor) were the
//             daemon's forwarding copies of the module's own names, and the
//             run engine — which lives INSIDE the module — was asking the host
//             to read the run's own decoration. What stays here is the host's
//             own state and the doors a MODULE needs without naming the runs
//             module (WithAgentProfile, WithMaxCost).
//             Code extracted from accessors.go during the Day-56 god-file split.

import (
	"context"
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
	"github.com/agezt/agezt/kernel/reflect"
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
// uses its own default of 4. Wraps the config field so the runner
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
// runner sub-package can read it through KernelAPI (Day 32).
func (k *Kernel) SkillForgeMinTools() int { return k.cfg.SkillForgeMinTools }

// The resume handshake's two CONTEXT facts — the owned marker and the seeded
// conversation — are the runs module's vocabulary (W3.76q). They live in
// kernel/modules/runs/resume, and the run engine reads them from there
// instead of asking the host for a copy: the daemon's ResumeOwnedKindFromCtx
// and ResumeSeedFromCtx doors are gone with them.

// DisableHeuristicBypass reports whether THIS run's effective
// config disables the deterministic heuristic bypass (time/date
// fast paths). Wraps the effectiveConfig field the Runner needs
// without leaking the full Config type across the package
// boundary (Day 34).
func (k *Kernel) DisableHeuristicBypass(ctx context.Context) bool {
	return k.effectiveConfig(ctx).DisableHeuristicBypass
}

// ResumeCheckpointFn returns the per-iteration checkpoint callback
// for a resumed run (M1002). Wraps the package-level resume.go
// helper so the runner package can drive it through KernelAPI.
func (k *Kernel) ResumeCheckpointFn(corr string) func(int, []llm.Message) {
	return k.resumeCheckpointFn(corr)
}

// ResolveRunModel picks the per-request model from the run ctx
// (explicit > daemon default with agent-profile overrides applied).
// Runs k.effectiveConfig internally so live daemon-default swaps
// (M816) + per-agent overrides (M783) are reflected without the
// Runner needing the Config type across the package boundary
// (Day 34).
func (k *Kernel) ResolveRunModel(ctx context.Context) (string, bool) {
	if m := runctx.ModelFromCtx(ctx); m != "" {
		return m, true
	}
	return k.effectiveConfig(ctx).Model, false
}

// WithAgentProfile binds a run context to a roster profile (M790): the soul
// becomes the system override, the model and its ordered fallbacks become the
// run's model chain, and the tool allow/deny lists, trust ceiling, noise
// policy and ledger identity ride along.
//
// The daemon keeps the DOOR and the run context owns the vocabulary: a host
// that holds a kernel handle asks the handle, so it never has to name the
// runs module (and never creates a cross-module edge to do it).
func (k *Kernel) WithAgentProfile(ctx context.Context, p agentapi.Profile) context.Context {
	return runctx.WithAgentProfile(ctx, p)
}

// WithMaxCost caps the cumulative provider spend (in USD-microcents) for the
// run started with it (M166) — the per-run cost analogue of WithRunTimeout.
// The daemon keeps the DOOR; the run context owns the rule.
func (k *Kernel) WithMaxCost(ctx context.Context, mc int64) context.Context {
	return runctx.WithMaxCost(ctx, mc)
}

// ShadowEval reports whether post-run shadow-skill judgement is
// enabled (SPEC-05 §5.2). When true, the run engine evaluates
// the shadow skills relevant to a completed run against what
// actually happened — opt-in, best-effort, no side effects (see
// MaybeShadowEval).
func (k *Kernel) ShadowEval() bool { return k.cfg.ShadowEval }
