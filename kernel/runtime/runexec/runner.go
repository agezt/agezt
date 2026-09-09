// SPDX-License-Identifier: MIT

package runexec

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/assure"
	"github.com/agezt/agezt/kernel/event"
	intentmodel "github.com/agezt/agezt/kernel/intent"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/resume"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/worldmodel"
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
func (r *Runner) RunWithRetry(ctx context.Context, corr, intent string, pol roster.RetryPolicy) (string, error) {
	max := pol.MaxAttempts
	if max <= 1 {
		return r.k.RunWith(ctx, corr, intent)
	}
	if max > 10 {
		max = 10
	}
	// Resume ticket ownership (M1002): the retry wrapper owns
	// one ticket across all attempts; the inner RunWith calls
	// reuse the corr and skip creation. The deferred finalize
	// reads the final named err, so a shutdown-interrupted
	// retry keeps its ticket while a genuine give-up deletes it.
	var owns bool
	ctx, owns = r.k.ClaimResumeTicket(ctx, corr, intent, resume.KindRetry, 0)
	var ans string
	var err error
	if owns {
		defer func() { r.k.FinalizeResumeTicket(corr, err) }()
	}
	var lastErr error
	for attempt := 1; attempt <= max; attempt++ {
		ans, err = r.k.RunWith(ctx, corr, intent)
		if err == nil {
			return ans, nil
		}
		lastErr = err
		reason := r.k.RetryReason(err)
		if attempt >= max || !r.k.AgentRetryable(reason, pol.RetryOn) {
			return "", err
		}
		delay := r.k.RetryDelay(pol, attempt)
		agentSlug := r.k.AgentSlugFromCtx(ctx)
		subject := "agent.retry"
		if agentSlug != "" {
			subject = "agent." + agentSlug + ".retry"
		}
		_, _ = r.k.Bus().Publish(event.Spec{
			Subject:       subject,
			Kind:          event.KindAgentRetry,
			Actor:         "agent-retry",
			CorrelationID: corr,
			Payload: map[string]any{
				"agent":          agentSlug,
				"attempt":        attempt,
				"next_attempt":   attempt + 1,
				"max_attempts":   max,
				"reason":         reason,
				"error":          err.Error(),
				"delay_ms":       int64(delay / time.Millisecond),
				"backoff":        strings.TrimSpace(pol.Backoff),
				"base_delay_sec": pol.BaseDelaySec,
				"max_delay_sec":  pol.MaxDelaySec,
				"retry_on":       append([]string{}, pol.RetryOn...),
			},
		})
		if delay <= 0 {
			continue
		}
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
			return "", ctx.Err()
		case <-t.C:
		}
	}
	return "", lastErr
}

// Why returns every event with the same correlation_id as the
// named event, in seq order (the M0.5 form of `agt why`).
func (r *Runner) Why(eventID string) ([]*event.Event, error) {
	return r.k.Journal().Why(eventID)
}

// Causes returns the causation ancestry of an event, root-first
// — the provenance walk SPEC-01 §7.1 describes.
func (r *Runner) Causes(eventID string) ([]*event.Event, error) {
	return r.k.Journal().Causes(eventID)
}

// ParentOf returns the lead run's correlation for a sub-agent
// run, or "" if childCorr was not spawned via delegation (M42).
func (r *Runner) ParentOf(childCorr string) string {
	return r.k.Journal().ParentOf(childCorr)
}

// Verify replays every event and confirms the BLAKE3 chain is
// intact. Returns nil on success.
func (r *Runner) Verify() error {
	return r.k.Journal().Verify()
}

// VerifyCompletion asks the model whether the given ANSWER
// fully accomplishes TASK. Returns an assure.Verdict with a
// complete/gap judgement (Day 33). The body was moved from
// kernel/runtime/runexec.go's private verifyCompletion. The
// bounded RunAssured loop and the workboard criteria check
// both consume this through *Kernel.VerifyCompletion.
func (r *Runner) VerifyCompletion(ctx context.Context, corr, task, answer string) (assure.Verdict, error) {
	prompt := "You are a strict completion checker. Given a TASK and the ANSWER an agent produced, decide whether the answer FULLY accomplishes the task with nothing important left undone. Be skeptical: a plan or a promise to do it is NOT completion.\n\n" +
		"Reply with ONLY a JSON object and no other text: {\"complete\": true|false, \"gap\": \"<concise description of what is still missing; empty string if complete>\"}.\n\n" +
		"TASK:\n" + task + "\n\nANSWER:\n" + answer
	resp, err := r.k.CompleteAux(ctx, corr, "verify", agent.CompletionRequest{
		Model:     r.k.Model(),
		MaxTokens: assureVerifyMaxTokens,
		Messages:  []agent.Message{{Role: agent.RoleUser, Content: prompt}},
	})
	if err != nil {
		return assure.Verdict{}, err
	}
	v, ok := assure.ParseVerdict(resp.Message.Content)
	if !ok {
		v = assure.Verdict{Complete: false, Gap: "verifier reply was not valid JSON"}
	}
	_, _ = r.k.Bus().Publish(event.Spec{
		Subject:       "agent.agent-" + corr + ".assure",
		Kind:          event.KindAssureVerdict,
		Actor:         "assure",
		CorrelationID: corr,
		Payload:       map[string]any{"complete": v.Complete, "gap": v.Gap},
	})
	return v, nil
}

// PublishHeuristicBypass journals a deterministic-look-up hit
// (what-time-is-it / today's-date) so the run's timeline still
// shows the task, the bypass, and the canned answer (Day 33).
// The body was moved from kernel/runtime/runexec.go's private
// publishHeuristicBypass.
func (r *Runner) PublishHeuristicBypass(ctx context.Context, corr, actor, intent, answer string) error {
	subject := func(suffix string) string {
		return "agent." + actor + "." + suffix
	}
	publish := func(kind event.Kind, suffix string, payload any) error {
		_, err := r.k.Bus().Publish(event.Spec{
			Subject:       subject(suffix),
			Kind:          kind,
			Actor:         actor,
			CorrelationID: corr,
			Payload:       payload,
		})
		return err
	}
	if err := publish(event.KindTaskReceived, "task", map[string]any{"intent": intent}); err != nil {
		return fmt.Errorf("runtime: publish heuristic task.received: %w", err)
	}
	if err := publish(event.KindInfo, "heuristic", map[string]any{
		"bypass": "deterministic",
		"reason": "known-safe fast path",
	}); err != nil {
		return fmt.Errorf("runtime: publish heuristic bypass: %w", err)
	}
	if err := publish(event.KindTaskCompleted, "task", map[string]any{
		"iters":   0,
		"chars":   len(answer),
		"stopped": "heuristic_bypass",
		"answer":  truncateHeuristicAnswer(answer),
	}); err != nil {
		return fmt.Errorf("runtime: publish heuristic task.completed: %w", err)
	}
	return nil
}

// ErrNoVisionModel is returned by DescribeImages when no vision-
// capable model is available. Defined in this package (separate
// identity from the canonical runtime.ErrNoVisionModel; same
// text). The *Kernel.DescribeImages wrapper translates via
// errors.Is so external callers see the canonical value.
var ErrNoVisionModel = errors.New("runtime: no vision-capable model available")

// DescribeImages runs the vision SIDECAR (M821): it sends the
// images to a keyed vision-capable model and returns a text
// description, so a run whose active model can't see images can
// still "read" them (Day 33). The body was moved from
// kernel/runtime/runexec.go.
func (r *Runner) DescribeImages(ctx context.Context, corr string, images []string, hint string) (string, error) {
	if len(images) == 0 {
		return "", nil
	}
	if r.k.VisionModel() == nil {
		return "", ErrNoVisionModel
	}
	model, ok := r.k.VisionModel()()
	if !ok || model == "" {
		return "", ErrNoVisionModel
	}
	prompt := hint
	if strings.TrimSpace(prompt) == "" {
		prompt = "Describe the attached image(s) in detail and transcribe any visible text. Be thorough and factual."
	}
	resp, err := r.k.CompleteAux(ctx, corr, "vision", agent.CompletionRequest{
		Model:     model,
		MaxTokens: visionDescribeMaxTokens,
		Messages:  []agent.Message{{Role: agent.RoleUser, Content: prompt, Images: images}},
	})
	if err != nil {
		return "", err
	}
	_, _ = r.k.Bus().Publish(event.Spec{
		Subject:       "agent.agent-" + corr + ".vision",
		Kind:          event.KindCapabilityRerouted,
		Actor:         "vision",
		CorrelationID: corr,
		Payload: map[string]any{
			"from_model": r.k.Model(),
			"to_model":   model,
			"capability": "vision",
			"images":     len(images),
		},
	})
	return resp.Message.Content, nil
}

// CompleteAgentLifecycle advances the durable lifecycle for a
// successful run (Day 33). Body moved from kernel/runtime/
// runexec.go's private completeAgentLifecycle. The Runner is
// the canonical caller; *Kernel.CompleteAgentLifecycle preserves
// the public surface for external callers (scheduled workflows,
// direct tool targets) that don't go through RunWith.
func (r *Runner) CompleteAgentLifecycle(ctx context.Context, corr string) {
	slug := r.k.AgentSlugFromCtx(ctx)
	if slug == "" {
		return
	}
	current, ok := r.k.Roster().Get(slug)
	if !ok || current.Retired {
		return
	}
	lifecycle := current.Lifecycle
	if shouldRetireAgentAfterComplete(lifecycle) {
		_, _ = r.k.SetProfileRetired(slug, true, "completed run "+corr)
		return
	}
	if strings.TrimSpace(lifecycle.Mode) != roster.LifecycleCycle && lifecycle.MaxCycles <= 0 {
		return
	}
	var completed, max int
	var advanced bool
	_, found, err := r.k.UpdateProfile(slug, func(p *roster.Profile) {
		// Idempotency: one logical run (correlation) advances the cycle
		// exactly once. RunAssured/RunWithRetry re-invoke RunWith
		// under the SAME corr (re-running until the work verifies
		// complete / a transient error clears); each inner success
		// calls here, so without this guard a single logical run would
		// double-count. The check sits inside the atomic UpdateProfile,
		// so it is race-free, and the marker is durable across
		// restarts. A correlation-less completion (corr=="") is never
		// guarded.
		if corr != "" && strings.TrimSpace(p.Lifecycle.LastCompletedRun) == corr {
			completed = p.Lifecycle.CompletedCycles
			max = p.Lifecycle.MaxCycles
			return
		}
		if strings.TrimSpace(p.Lifecycle.Mode) == "" {
			p.Lifecycle.Mode = roster.LifecycleCycle
		}
		p.Lifecycle.CompletedCycles++
		if corr != "" {
			p.Lifecycle.LastCompletedRun = corr
		}
		resetCompletedCycleTasks(p.TaskList)
		completed = p.Lifecycle.CompletedCycles
		max = p.Lifecycle.MaxCycles
		advanced = true
	})
	if err != nil || !found || !advanced {
		return
	}
	_, _ = r.k.Bus().Publish(event.Spec{
		Subject:       "roster." + slug,
		Kind:          event.KindRosterUpdated,
		Actor:         "roster",
		CorrelationID: corr,
		Payload: map[string]any{
			"slug":             slug,
			"action":           "lifecycle_cycle_completed",
			"completed_cycles": completed,
			"max_cycles":       max,
			"run":              corr,
		},
	})
	if max > 0 && completed >= max {
		_, _ = r.k.SetProfileRetired(slug, true, fmt.Sprintf("completed %d/%d cycles on run %s", completed, max, corr))
	}
}

// RunAssured is the "do-it-for-sure" loop (M651). Body migrated
// from kernel/runtime/runexec.go (Day 33). Same Resume-ticket
// ownership dance as before; the inner RunWith / RunWithRetry
// calls already route through the Runner (or *Kernel for
// RunWith, which still lives on the host — Day 23+ documented
// gap).
func (r *Runner) RunAssured(ctx context.Context, corr, intent string, maxAttempts int) (string, assure.Result, error) {
	ctx, owns := r.k.ClaimResumeTicket(ctx, corr, intent, resume.KindAssured, maxAttempts)
	res, err := assure.Until(ctx, intent, maxAttempts,
		func(ctx context.Context, _ int, task string) (string, error) {
			if pol, ok := r.k.AgentRetryPolicyFromCtx(ctx); ok && pol.MaxAttempts > 1 {
				return r.k.RunWithRetry(ctx, corr, task, pol)
			}
			return r.k.RunWith(ctx, corr, task)
		},
		func(ctx context.Context, task, answer string) (assure.Verdict, error) {
			return r.VerifyCompletion(ctx, corr, task, answer)
		},
	)
	if owns {
		r.k.FinalizeResumeTicket(corr, err)
	}
	return res.Answer, res, err
}

// MaybeDistill folds the run's journal and, if the run made at
// least MemoryDistillMinTools tool calls, runs one best-effort
// distillation pass over a compact transcript (Day 32). Best-
// effort: a failure is journaled as a memory distill error and
// swallowed — distillation must not turn a successful task into
// a failed one. Body moved from kernel/runtime/runexec.go.
func (r *Runner) MaybeDistill(ctx context.Context, corr, intent, answer string) {
	minTools := r.k.MemoryDistillMinTools()
	if minTools <= 0 {
		minTools = 4
	}
	toolCount, names := r.k.FoldRunTools(corr)
	if toolCount < minTools {
		return
	}
	transcript := buildTranscript(names, answer)
	if _, err := r.k.Memory().Distill(ctx, corr, r.k.Provider(), r.k.Model(), intent, transcript); err != nil {
		_, _ = r.k.Bus().Publish(event.Spec{
			Subject:       "memory.distill_failed",
			Kind:          event.KindMemoryWritten,
			Actor:         "memory",
			CorrelationID: corr,
			Payload:       map[string]any{"action": "distill_failed", "error": err.Error()},
		})
	}
}

// MaybeForge proposes a DRAFT skill from the run's journal fold
// when the run made at least SkillForgeMinTools tool calls (Day 32).
// The operator promotes DRAFT skills — §5.1/§5.3. Same
// threshold-gated, never-fail-the-task contract as MaybeDistill.
func (r *Runner) MaybeForge(ctx context.Context, corr, intent, answer string) {
	minTools := r.k.SkillForgeMinTools()
	if minTools <= 0 {
		minTools = 4
	}
	toolCount, names := r.k.FoldRunTools(corr)
	if toolCount < minTools {
		return
	}
	transcript := buildTranscript(names, answer)
	if _, err := r.k.Forge().Propose(ctx, corr, r.k.Provider(), r.k.Model(), intent, transcript); err != nil {
		_, _ = r.k.Bus().Publish(event.Spec{
			Subject:       "skill.propose_failed",
			Kind:          event.KindSkillCreated,
			Actor:         "forge",
			CorrelationID: corr,
			Payload:       map[string]any{"action": "propose_failed", "error": err.Error()},
		})
	}
}

// MaybeShadowEval judges the shadow skills relevant to a just-
// completed run (SPEC-05 §5.2, Day 32). Best-effort: a judge
// failure is journaled but never affects the run, which has
// already returned its answer.
func (r *Runner) MaybeShadowEval(ctx context.Context, corr, intent, answer string) {
	if err := r.k.Forge().ShadowEvaluate(ctx, corr, r.k.Provider(), r.k.Model(), intent, answer, shadowEvalLimit); err != nil {
		_, _ = r.k.Bus().Publish(event.Spec{
			Subject:       "skill.shadow_eval_failed",
			Kind:          event.KindSkillShadowEval,
			Actor:         "forge",
			CorrelationID: corr,
			Payload:       map[string]any{"error": err.Error()},
		})
	}
}

// errStubOnly is the sentinel returned by stub entry points
// until a future slice fills the body. It is a distinctive
// error so callers fail loudly instead of silently getting a
// zero-value answer. Currently unused (Day 23 filled all
// planned entry points) but kept as a guard against silent
// regressions on future KernelAPI additions.
var errStubOnly = errors.New("runexec: Runner entry point not yet implemented")

// keep package-level references live so the imports in this
// file stay used after the body moves settle. Removing them
// would force a churn round for the next refactor slice.
var (
	_ = agent.RoleUser
	_ = assure.Result{}
)
