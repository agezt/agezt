// SPDX-License-Identifier: MIT

// Runner core: New + Run + RunWith (the main entry points).
// Code extracted from runner.go during the Day-63 god-file split. Public API unchanged.
package runexec


import (
	"context"
	"github.com/agezt/agezt/kernel/agent"
	intentmodel "github.com/agezt/agezt/kernel/intent"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/resume"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/worldmodel"
	"time"
)



// Runner owns the kernel's run engine. The Runner speaks to
// the host through the KernelAPI interface, the same pattern
// lifecycle.Manager and accessors.Accessor use.
//
// Lock-ordering invariant: every site that takes a mutex must
// honour the order documented on the Kernel struct
//
//	configMu < runsMu < fanoutMu < treeMu < steersMu < spawnsMu < mcpMu
//
// Day 22's Run / RunAssured don't take any of those mutexes
// directly. Day 23 adds RunWithRetry (no mutex — just ctx /
// journal / bus / resume-ticket calls, all reachable through
// KernelAPI) and the journal re-exports (Why / Causes /
// ParentOf / Verify, all delegations to the host's journal).
//
// RunWith body stays on *Kernel for Day 23. The body needs
// ~30 private field/method accesses (k.halted, k.runs, k.runsMu,
// k.steers, k.fanout, k.tree, k.spawns and the matching mutexes,
// k.claimResumeTicket, k.completeAux, k.buildLoopConfig, the
// ctxKey* values, the runctx helpers, etc.). The runexec
// package cannot import kernel/runtime (cycle — kernel/runtime
// already imports runexec for Runner construction), so the
// only paths to move RunWith are (a) bloat the KernelAPI
// interface to ~80 metot, or (b) break the cycle by relocating
// the construction site out of compose.go. Both are out of
// scope for Day 23's lock-ordering refactor; the day ships
// the methods that are reachable today and records the gap
// for a future slice.
type Runner struct {
	k KernelAPI
}

// New constructs the Runner. k is the host kernel; the Runner
// is a stateless value around it.
func New(k KernelAPI) *Runner { return &Runner{k: k} }

// Run executes one tool-loop end-to-end and returns (answer,
// corr, err). It mints a correlation ID internally; for the
// subscribe-then-run flow the control plane uses, see
// NewCorrelation + RunWith.
func (r *Runner) Run(ctx context.Context, intent string) (string, string, error) {
	corr := r.k.NewCorrelation()
	ans, err := r.RunWith(ctx, corr, intent)
	return ans, corr, err
}

// (RunAssured body is defined further down — the canonical version
// holds the full migration; this comment is here because the Day 22
// stub used to sit at this position.)

// RunWith is the linear run-engine entry point. The 260-line
// body stays on *Kernel (see type comment above); the Runner
// delegates so callers targeting the new surface compile
// unchanged.
func (r *Runner) RunWith(ctx context.Context, corr, intent string) (string, error) {
	runCtx, cancel, steer, err := r.k.SetupRunState(corr, ctx)
	if err != nil {
		return "", err
	}
	_ = steer // wired into LoopConfig.Steer further down

	// Deferred cleanup (Day 34): the 5-mutex dance is encapsulated
	// in r.k.CleanupRunState; we just iterate the orphan cancels
	// and call the runCtx cancel.
	defer func() {
		orphans := r.k.CleanupRunState(corr)
		for _, c := range orphans {
			c()
		}
		cancel()
	}()

	actor := "agent-" + corr
	// Stash actor + correlation on the ctx so the policyHook can
	// thread them into approval.Submit (the agent.Policy contract
	// doesn't expose them directly), and so the in-process memory tool
	// can journal its writes under this run.
	runCtx = r.k.WithActorCorrelation(runCtx, actor, corr)
	runCtx = memory.WithCorrelation(runCtx, corr)
	runCtx = worldmodel.WithCorrelation(runCtx, corr)
	runCtx = skill.WithCorrelation(runCtx, corr)
	systemAgent := r.k.SystemAgentFromCtx(runCtx)
	skillDirective := skill.ParseActivationDirective(intent)
	if skillDirective.Explicit && skillDirective.CleanIntent != "" {
		intent = skillDirective.CleanIntent
	}
	intentFrame, ok := intentmodel.FrameFromContext(runCtx)
	if !ok {
		intentFrame = intentmodel.Interpret(intent)
		runCtx = intentmodel.WithFrame(runCtx, intentFrame)
	}
	r.k.PublishIntentInterpreted(corr, actor, intentFrame)
	// So warden-backed tools (shell) stamp this run's correlation onto their
	// warden.executed events — making the isolation profile show up in the run's
	// timeline and walkable by `agt why`.
	runCtx = warden.WithCorrelation(runCtx, corr)

	if !r.k.DisableHeuristicBypass(runCtx) {
		if answer, ok := deterministicHeuristicBypass(intent, time.Now()); ok {
			if err := r.k.PublishHeuristicBypass(runCtx, corr, actor, intent, answer); err != nil {
				return "", err
			}
			r.k.CompleteAgentLifecycle(runCtx, corr)
			return answer, nil
		}
	}

	// System-prompt assembly: per-run override or live default, then
	// profile/taste/memory/world/skill injection (buildRunPrompt).
	system, activatedSkillIDs := r.k.BuildRunPrompt(runCtx, corr, actor, intent, systemAgent, skillDirective)

	model, modelExplicit := r.k.ResolveRunModel(runCtx)
	// An EXPLICIT pick must actually serve the run (M931): carry the pick as
	// the per-request chain, which wins over the task chain (M787 precedence).
	modelChain := r.k.ModelChainFromCtx(runCtx)
	if len(modelChain) == 0 && modelExplicit {
		modelChain = []string{model}
	}

	// Governance- and capacity-shaped LoopConfig fields — shared verbatim with
	// executeSubAgent so root and delegated runs can never diverge on cost
	// accounting, compaction, or tool policy (LD-1).
	lc := r.k.BuildLoopConfig(runCtx, corr, model)

	// Host-environment preamble (M609) — see injectHostEnvironment.
	system = r.k.InjectHostEnvironment(system, lc.Tools)

	// Durable resume (M1002): a root run owns a ticket unless a governed wrapper
	// (RunAssured/RunWithRetry) or the resumer already created one for this corr.
	var ownedHere bool
	runCtx, ownedHere = r.k.ClaimResumeTicket(runCtx, corr, intent, resume.KindRun, 0)
	var resumeCheckpoint func(int, []agent.Message)
	var resumePriorMessages []agent.Message
	var resumeStartIter int
	if kind, owned := r.k.ResumeOwnedKindFromCtx(runCtx); ownedHere || (owned && kind == resume.KindRun) {
		resumeCheckpoint = r.k.ResumeCheckpointFn(corr)
		if msgs, it, ok := r.k.ResumeSeedFromCtx(runCtx); ok {
			resumePriorMessages, resumeStartIter = msgs, it
		}
	}

	// Run identity + root-run-only concerns layered on the shared base.
	lc.TaskType = "chat"       // M703: main agent loop → "chat" routing target
	lc.ModelChain = modelChain // M787 agent fallbacks, or the explicit pick (M931)
	lc.Agent = r.k.AgentSlugFromCtx(runCtx)
	lc.AgentDailyCeilingMc = r.k.AgentDailyMcFromCtx(runCtx)
	lc.WakeSource = r.k.WakeContextSource(runCtx)
	lc.WakeReason = r.k.WakeContextReason(runCtx)
	lc.ScheduleID = r.k.WakeContextScheduleID(runCtx)
	lc.StandingID = r.k.WakeContextStandingID(runCtx)
	lc.StandingName = r.k.WakeContextStandingName(runCtx)
	lc.TriggerSubject = r.k.WakeContextTriggerSubject(runCtx)
	lc.ParentCorrelation = r.k.WakeContextParentCorrelation(runCtx)
	lc.System = system
	lc.Actor = actor
	lc.CorrelationID = corr
	lc.Images = r.k.ImagesFromCtx(runCtx)                // M93: image attachments
	lc.JSONMode = r.k.JSONModeFromCtx(runCtx)            // M314: structured-output request
	lc.MaxRunCostMicrocents = r.k.MaxCostFromCtx(runCtx) // M166: per-run cost cap
	lc.Steer = steer                                // M608: live operator steering
	lc.Checkpoint = resumeCheckpoint                 // M1002: persist snapshot each iteration
	lc.PriorMessages = resumePriorMessages           // M1002: seed a resumed run's conversation
	lc.StartIter = resumeStartIter                   // M1002: continue iter numbering on resume
	answer, err := agent.Run(runCtx, lc, intent)

	// Resume ticket (M1002): clear it on a clean/failed/cancelled terminal, but
	// keep it if the run was interrupted by shutdown.
	if ownedHere {
		r.k.FinalizeResumeTicket(corr, err)
	}

	// Deregister the steering control the instant the agent loop returns.
	r.k.DeregisterRunSteer(corr)

	// Attribute the run's outcome to the skills it activated.
	if r.k.Forge() != nil && len(activatedSkillIDs) > 0 {
		r.k.Forge().RecordOutcome(corr, activatedSkillIDs, err == nil)
	}

	if err != nil {
		r.k.PublishContextFailureAnalysis(corr, actor, err)
		return answer, err
	}

	// Auto-distillation: after a multi-tool run, extract durable facts.
	if r.k.MemoryDistill() && !systemAgent {
		r.k.MaybeDistill(runCtx, corr, intent, answer)
	}
	// Forge proposal: propose a DRAFT skill.
	if r.k.SkillForge() && !systemAgent {
		r.k.MaybeForge(runCtx, corr, intent, answer)
	}
	// Shadow-evaluate relevant shadow skills.
	if r.k.ShadowEval() && !systemAgent && r.k.Forge() != nil {
		r.k.MaybeShadowEval(runCtx, corr, intent, answer)
	}
	r.k.CompleteAgentLifecycle(runCtx, corr)
	return answer, nil
}

// RunWithRetry executes one agent run using the profile's
// failure retry policy. Distinct from provider retry (one LLM
// request) and RunAssured (semantic completion verification):
// retries the whole governed run after a terminal error,
// journaling each retry decision under the same correlation.
//
// Day 23: body moved here from kernel/runtime/runexec.go. The
// Runner drives the resume-ticket lifecycle through KernelAPI
// (ClaimResumeTicket / FinalizeResumeTicket) and reads the
// retry policy / agent slug from the host's context helpers
// (AgentRetryPolicyFromCtx / AgentSlugFromCtx). No mutex is
// taken directly — the body is journal/bus/timer-only.