// SPDX-License-Identifier: MIT

package runner

// Provenance: Runner core: New + Run + RunWith (the main entry points). Code
//             extracted from runner.go during the Day-63 god-file split. Public API
//             unchanged.

import (
	"context"
	"errors"
	"time"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
	intentmodel "github.com/agezt/agezt/kernel/intent"
	agentsapi "github.com/agezt/agezt/kernel/modules/agents/api"
	"github.com/agezt/agezt/kernel/modules/runs/agent"
	"github.com/agezt/agezt/kernel/modules/runs/api"
	"github.com/agezt/agezt/kernel/modules/runs/prompt"
	"github.com/agezt/agezt/kernel/modules/runs/publish"
	"github.com/agezt/agezt/kernel/modules/runs/resume"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
	"github.com/agezt/agezt/kernel/modules/runs/state"
	"github.com/agezt/agezt/kernel/warden"
)

// Runner owns the kernel's run engine. The Runner speaks to
// the host through the KernelAPI interface, the same pattern
// lifecycle.Manager and accessors.Accessor use.
//
// Lock-ordering invariant: every site that takes a mutex must
// honour the order documented on the Kernel struct
//
//	registry lock < configMu < the live-run state's own lock
//
// The live-run state's own lock is the MODULE's own (W3.76ae): the
// steering registry and the per-run registration live in
// kernel/modules/runs/state, and that package takes its lock INSIDE the
// registry's lock. The engine asks the module's state package for the
// run's setup and cleanup; it does not ask the host for them.
//
// The engine's body needs the host's own bookkeeping through KernelAPI
// (the resume ticket, the prompt assembly, the roster, the retry policy)
// and the module's own state for the run's live state. That split is
// deliberate: the host keeps what is the HOST's vocabulary, the module
// keeps what is the MODULE's.
type Runner struct {
	k KernelAPI

	// roster is the AGENTS MODULE's roster service (W3.76o).
	//
	// The run engine writes the roster's rows — it retires an agent whose run
	// completed and advances the agent's cycle counter — and those writes are
	// the agents module's operations, not the host's. Asking the host to ask
	// the module was a hop that bought nothing: the run engine may import
	// another module's api package, so it asks the module itself.
	//
	// nil means the host wired no roster service: the run engine simply has
	// no roster to write, which is the same answer the host gave.
	roster agentsapi.RosterService

	// runState is the RUNS MODULE's live-run state (W3.76ae): the steering
	// registry and the per-run registration.
	//
	// The run's setup and cleanup used to be the host's own verbs, and the
	// engine asked the host for them through its port — a round trip inside
	// one module. The state is the module's, so the engine asks the module.
	//
	// nil means the host wired no live-run state: the engine cannot start a
	// run, and it says so instead of registering nothing.
	runState *state.State

	// prompt is the RUNS MODULE's prompt assembly (W3.76af): the layer that
	// decides what a run knows before it starts — the live identity, the
	// operator profile, the taste exemplars, the recalled memory, the
	// resolved world entities and the activated skills — plus the
	// host-environment preamble.
	//
	// The daemon held two verbs over it (BuildRunPrompt,
	// InjectHostEnvironment) and the run engine asked the HOST for them
	// through its port — a round trip inside ONE module: the assembly was
	// already the module's, and the delegation engine (which lives next to
	// the run engine) held the very same service. The engine asks the module
	// now.
	//
	// nil means the host wired no prompt service: the engine cannot assemble
	// a prompt, and it says so instead of running a bare one.
	prompt *prompt.Service

	// publish is the RUNS MODULE's publish surface (W3.76ag): what the run
	// says about itself — the reading it made of the user's utterance, and
	// the analysis of what its context selection left out.
	//
	// The daemon held two verbs over them (PublishIntentInterpreted,
	// PublishContextFailureAnalysis) and the run engine asked the HOST for
	// them through its port — a round trip inside ONE module. The engine asks
	// the module now.
	//
	// nil means the host wired no publisher: the engine cannot say anything
	// about itself, and it says so instead of running silently.
	publish *publish.Publisher

	// tickets is the RUNS MODULE's ticket lifecycle (W3.76ah): who owns the
	// durable resume ticket, when it is refreshed and when it is deleted.
	//
	// The daemon held three verbs over it (ClaimResumeTicket,
	// FinalizeResumeTicket, ResumeCheckpointFn) and the run engine asked the
	// HOST for them through its port — a round trip inside ONE module: the
	// tickets, the store and the rules that judge them are the module's own.
	// The engine asks the module now.
	//
	// nil means the host wired no ticket lifecycle: the engine cannot say
	// whether this run carries a ticket, and it says so instead of running a
	// run whose ticket it does not own.
	tickets *resume.Service
}

// The run engine answers the module's own PORT (W3.76ac): the run engine is
// the runs module's, and the port names it once. The compiler says so, not a
// comment.
var _ api.RunnerService = (*Runner)(nil)

// New constructs the Runner. k is the host kernel; the Runner
// is a stateless value around it.
//
// roster is the agents module's roster service — the module the run engine
// asks when it writes the roster (W3.76o). runState is the module's own
// live-run state — the module the run engine asks when it registers and
// releases a run (W3.76ae). promptSvc is the module's own prompt assembly —
// the module the run engine asks when it builds a run's prompt (W3.76af).
// publisher is the module's own publish surface — the module the run engine
// asks when it says something about itself (W3.76ag). ticketSvc is the
// module's own ticket lifecycle — the module the run engine asks when it
// claims, refreshes and finalizes the run's durable ticket (W3.76ah).
func New(k KernelAPI, roster agentsapi.RosterService, runState *state.State, promptSvc *prompt.Service, publisher *publish.Publisher, ticketSvc *resume.Service) *Runner {
	return &Runner{k: k, roster: roster, runState: runState, prompt: promptSvc, publish: publisher, tickets: ticketSvc}
}

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
	// A run without a correlation id cannot be steered, observed or
	// resumed, so the run engine refuses it here — at the implementation.
	// The daemon's own door checks it too (the host's door for the callers
	// above), and the run engine no longer asks the host for this.
	if corr == "" {
		return "", errors.New("runner: correlation id required")
	}
	if r.runState == nil {
		return "", errors.New("runner: the live-run state is not available")
	}
	// The prompt assembly is the MODULE's own (W3.76af): the engine holds the
	// module's service and asks it. A host that wired none leaves the engine
	// quiet — it says the assembly is missing instead of running a bare
	// prompt — and it says it BEFORE the run is registered, so a run that
	// cannot be assembled leaves no live-run state behind.
	if r.prompt == nil {
		return "", errors.New("runner: the prompt assembly is not available")
	}
	// The PUBLISH SURFACE is the MODULE's own (W3.76ag): the engine holds the
	// module's publisher and asks it, instead of asking the host for a verb
	// over the module's own body. A host that wired none leaves the engine
	// quiet — and it says so BEFORE the run is registered, so a run that
	// cannot say anything about itself leaves no live-run state behind.
	if r.publish == nil {
		return "", errors.New("runner: the publish surface is not available")
	}
	// The TICKET LIFECYCLE is the MODULE's own (W3.76ah): the engine holds
	// the module's service and asks it, instead of asking the host for a verb
	// over the module's own body. A host that wired none leaves the engine
	// quiet — and it says so BEFORE the run is registered, so a run whose
	// ticket cannot be owned leaves no live-run state behind.
	if r.tickets == nil {
		return "", errors.New("runner: the ticket lifecycle is not available")
	}
	runCtx, cancel, steer, err := r.runState.SetupRunState(corr, ctx)
	if err != nil {
		return "", err
	}
	_ = steer // wired into LoopConfig.Steer further down

	// Deferred cleanup (Day 34): the 5-mutex dance is encapsulated
	// in the module's live-run state; we just iterate the orphan cancels
	// and call the runCtx cancel.
	defer func() {
		orphans := r.runState.CleanupRunState(corr)
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
	runCtx = runctx.WithActor(runCtx, actor, corr)
	runCtx = memoryapi.WithCorrelation(runCtx, corr)
	runCtx = opapi.WithCorrelation(runCtx, corr)
	systemAgent := runctx.SystemAgentFromCtx(runCtx)
	skillDirective := skillapi.ParseActivationDirective(intent)
	if skillDirective.Explicit && skillDirective.CleanIntent != "" {
		intent = skillDirective.CleanIntent
	}
	intentFrame, ok := intentmodel.FrameFromContext(runCtx)
	if !ok {
		intentFrame = intentmodel.Interpret(intent)
		runCtx = intentmodel.WithFrame(runCtx, intentFrame)
	}
	r.publish.IntentInterpreted(corr, actor, intentFrame)
	// So warden-backed tools (shell) stamp this run's correlation onto their
	// warden.executed events — making the isolation profile show up in the run's
	// timeline and walkable by `agt why`.
	runCtx = warden.WithCorrelation(runCtx, corr)

	if !r.k.DisableHeuristicBypass(runCtx) {
		if answer, ok := deterministicHeuristicBypass(intent, time.Now()); ok {
			if err := r.PublishHeuristicBypass(runCtx, corr, actor, intent, answer); err != nil {
				return "", err
			}
			r.CompleteAgentLifecycle(runCtx, corr)
			return answer, nil
		}
	}

	// System-prompt assembly: per-run override or live default, then
	// profile/taste/memory/world/skill injection.
	//
	// The assembly is the RUNS MODULE's own (W3.76af): the engine holds the
	// module's prompt service and asks it, instead of asking the host for a
	// verb over the module's own body.
	system, activatedSkillIDs := r.prompt.Build(runCtx, corr, actor, intent, systemAgent, skillDirective)

	model, modelExplicit := r.k.ResolveRunModel(runCtx)
	// An EXPLICIT pick must actually serve the run (M931): carry the pick as
	// the per-request chain, which wins over the task chain (M787 precedence).
	modelChain := runctx.ModelChainFromCtx(runCtx)
	if len(modelChain) == 0 && modelExplicit {
		modelChain = []string{model}
	}

	// Governance- and capacity-shaped LoopConfig fields — shared verbatim with
	// executeSubAgent so root and delegated runs can never diverge on cost
	// accounting, compaction, or tool policy (LD-1).
	lc := r.k.BuildLoopConfig(runCtx, corr, model)

	// Host-environment preamble (M609) — the module's own assembly.
	system = r.prompt.BuildEnvironment(system, lc.Tools)

	// Durable resume (M1002): a root run owns a ticket unless a governed wrapper
	// (RunAssured/RunWithRetry) or the resumer already created one for this corr.
	// The lifecycle is the MODULE's own (W3.76ah): the engine asks the module.
	var ownedHere bool
	runCtx, ownedHere = r.tickets.Claim(runCtx, corr, intent, resume.KindRun, 0)
	var resumeCheckpoint func(int, []llm.Message)
	var resumePriorMessages []llm.Message
	var resumeStartIter int
	if kind, owned := resume.OwnedKindFromCtx(runCtx); ownedHere || (owned && kind == resume.KindRun) {
		resumeCheckpoint = r.tickets.CheckpointFn(corr)
		if msgs, it, ok := resume.SeedFromCtx(runCtx); ok {
			resumePriorMessages, resumeStartIter = msgs, it
		}
	}

	// Run identity + root-run-only concerns layered on the shared base.
	lc.TaskType = "chat"       // M703: main agent loop → "chat" routing target
	lc.ModelChain = modelChain // M787 agent fallbacks, or the explicit pick (M931)
	lc.Agent = runctx.AgentSlugFromCtx(runCtx)
	lc.AgentDailyCeilingMc = runctx.AgentDailyMcFromCtx(runCtx)
	wake := runctx.WakeContextFromCtx(runCtx)
	lc.WakeSource = wake.Source
	lc.WakeReason = wake.Reason
	lc.ScheduleID = wake.ScheduleID
	lc.StandingID = wake.StandingID
	lc.StandingName = wake.StandingName
	lc.TriggerSubject = wake.TriggerSubject
	lc.ParentCorrelation = wake.ParentCorrelation
	lc.System = system
	lc.Actor = actor
	lc.CorrelationID = corr
	lc.Images = runctx.ImagesFromCtx(runCtx)                // M93: image attachments
	lc.JSONMode = runctx.JSONModeFromCtx(runCtx)            // M314: structured-output request
	lc.MaxRunCostMicrocents = runctx.MaxCostFromCtx(runCtx) // M166: per-run cost cap
	lc.Steer = steer                                        // M608: live operator steering
	lc.Checkpoint = resumeCheckpoint                        // M1002: persist snapshot each iteration
	lc.PriorMessages = resumePriorMessages                  // M1002: seed a resumed run's conversation
	lc.StartIter = resumeStartIter                          // M1002: continue iter numbering on resume
	answer, err := agent.Run(runCtx, lc, intent)

	// Resume ticket (M1002): clear it on a clean/failed/cancelled terminal, but
	// keep it if the run was interrupted by shutdown.
	if ownedHere {
		r.tickets.Finalize(corr, err)
	}

	// Deregister the steering control the instant the agent loop returns.
	r.runState.DeregisterRunSteer(corr)

	// Attribute the run's outcome to the skills it activated.
	if r.k.Forge() != nil && len(activatedSkillIDs) > 0 {
		r.k.Forge().RecordOutcome(corr, activatedSkillIDs, err == nil)
	}

	if err != nil {
		r.publish.ContextFailureAnalysis(corr, actor, err)
		return answer, err
	}

	// Auto-distillation: after a multi-tool run, extract durable facts.
	if r.k.MemoryDistill() && !systemAgent {
		r.MaybeDistill(runCtx, corr, intent, answer)
	}
	// Forge proposal: propose a DRAFT skill.
	if r.k.SkillForge() && !systemAgent {
		r.MaybeForge(runCtx, corr, intent, answer)
	}
	// Shadow-evaluate relevant shadow skills.
	if r.k.ShadowEval() && !systemAgent && r.k.Forge() != nil {
		r.MaybeShadowEval(runCtx, corr, intent, answer)
	}
	r.CompleteAgentLifecycle(runCtx, corr)
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
