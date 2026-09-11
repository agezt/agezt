// SPDX-License-Identifier: MIT

// Sub-agent execution engine: prepareSubAgent (context window assembly), validateDelegationManager, executeSubAgent (run loop).
// Code extracted from subagent.go during the Day-40 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/delegation"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
)


func (k *Kernel) prepareSubAgent(ctx context.Context, task, model, taskType, agentRef string, async bool) (*subAgentPrep, error) {
	task = strings.TrimSpace(task)
	if task == "" {
		return nil, errors.New("task required")
	}
	// Delegate AS a named agent (M784): resolve the roster profile up front so
	// an unknown or paused agent is a clear tool error the lead adapts to, not
	// a sub-agent silently running as the default identity. The profile's
	// model/task type/cost ceiling apply as DEFAULTS; explicit call args win.
	var prof *roster.Profile
	if agentRef = strings.TrimSpace(agentRef); agentRef != "" {
		p, ok := k.roster.Get(agentRef)
		if !ok {
			return nil, fmt.Errorf("unknown agent %q (agt agent list)", agentRef)
		}
		if p.Retired {
			return nil, fmt.Errorf("agent %q is retired — revive it first (agt agent revive %s)", p.Slug, p.Slug)
		}
		if !p.Enabled {
			return nil, fmt.Errorf("agent %q is paused (agt agent resume %s)", p.Slug, p.Slug)
		}
		caller := agent.AgentFromContext(ctx)
		if !p.AllowsDelegationFrom(caller) {
			manager := strings.TrimSpace(p.ParentAgent)
			if manager == "" {
				manager = strings.TrimSpace(p.OwnerAgent)
			}
			return nil, fmt.Errorf("agent %q is managed by %q and cannot be delegated by %q", p.Slug, manager, caller)
		}
		if !p.AllowsDirectCall() {
			if err := k.validateDelegationManager(p, caller); err != nil {
				return nil, err
			}
		}
		prof = &p
	}
	// Per-sub-agent model (M705): an explicit model overrides the daemon default
	// for this delegation; an explicit task_type selects the routing chain whose
	// models provide the fallbacks (defaulting to "delegate"). Both are optional —
	// a bare delegate behaves exactly as before.
	model = strings.TrimSpace(model)
	taskType = strings.TrimSpace(taskType)
	if prof != nil {
		if model == "" {
			model = strings.TrimSpace(prof.Model)
		}
		if model == "" {
			if override, ok := agentConfigOverrideRaw(prof.ConfigOverrides, "AGEZT_MODEL"); ok {
				if parsed, ok := agentConfigStringValue(override); ok {
					model = parsed
				}
			}
		}
		if taskType == "" {
			taskType = strings.TrimSpace(prof.TaskType)
		}
	}
	if taskType == "" {
		taskType = "delegate"
	}
	// The live daemon default (k.Model(), hot-swapped on provider reload — M816),
	// not the boot seed: a delegation falling back to the boot model would keep
	// asking for whatever the daemon started with, which after a key rotation is
	// typically the model that just stopped being servable.
	defaultModel := k.Model()
	subModel := defaultModel
	if model != "" {
		subModel = model
	}
	if k.IsHalted() {
		return nil, ErrHalted
	}

	// Build the effective model chain (M787): the chosen model first, then the
	// profile's ordered fallbacks. Restrict to KEYED models (M838 bugfix) — an
	// unkeyed explicit model or fallback would fail to route mid-delegation, so
	// drop it; if nothing keyed remains, fall back to the daemon default. Done
	// HERE (before the spawn is journaled and the child runs) so the recorded and
	// executed model is the one actually used. No-op when ModelAvailable is unset.
	var modelChain []string
	if prof != nil && len(prof.Fallbacks) > 0 {
		modelChain = []string{subModel}
		for _, m := range prof.Fallbacks {
			if m = strings.TrimSpace(m); m != "" && m != subModel {
				modelChain = append(modelChain, m)
			}
		}
	}
	if avail := k.cfg.ModelAvailable; avail != nil {
		subModel, modelChain = delegation.KeyedModelChain(subModel, modelChain, avail, defaultModel)
	}

	depth := delegation.DepthFromCtx(ctx)
	maxDepth := k.cfg.SubAgentMaxDepth
	if maxDepth <= 0 {
		maxDepth = 1
	}
	if depth >= maxDepth {
		return nil, fmt.Errorf("max sub-agent depth %d reached", maxDepth)
	}

	parentCorr := correlationFromCtx(ctx)

	// Fan-out bound (M46): depth caps how DEEP delegation nests; this caps how
	// WIDE a single run fans out. We tally sub-agents per spawning correlation
	// (the lead run, or a sub-agent that itself delegates) in k.fanout and
	// refuse the Nth+1 call with a tool error the lead adapts to. 0 = unbounded
	// (default). A correlation-less spawn (no run context) can't be attributed,
	// so it's left unbounded. The tally is released when the spawning run ends
	// (RunWith's defer for top-level; this function's defer for a nested
	// spawner's own child correlation below).
	if maxFanout := k.cfg.SubAgentMaxFanout; maxFanout > 0 && parentCorr != "" {
		k.fanoutMu.Lock()
		n := k.fanout[parentCorr]
		if n >= maxFanout {
			k.fanoutMu.Unlock()
			return nil, fmt.Errorf("max sub-agent fan-out %d reached", maxFanout)
		}
		k.fanout[parentCorr] = n + 1
		k.fanoutMu.Unlock()
	}

	// Tree-total bound (M629): depth caps how DEEP and fan-out caps how WIDE at
	// one level, but a depth-D, fan-out-F tree can still hold up to F^D agents —
	// neither cap bounds the WHOLE tree's size. This caps the total sub-agents
	// across every depth of one delegation tree, attributed to the root run's
	// correlation. The root is the top-level lead (rootFromCtx is empty at depth
	// 0, so the lead's own correlation seeds the root); every descendant inherits
	// it via childCtx below, so a spawn three levels down still counts against
	// the same root. 0 = unbounded. The tally is released when the root run ends
	// (RunWith's defer deletes k.tree[corr]).
	rootCorr := rootFromCtx(ctx)
	if rootCorr == "" {
		rootCorr = parentCorr // this spawner is the tree root
	}
	if maxTotal := k.cfg.SubAgentMaxTotal; maxTotal > 0 && rootCorr != "" {
		k.treeMu.Lock()
		n := k.tree[rootCorr]
		if n >= maxTotal {
			k.treeMu.Unlock()
			return nil, fmt.Errorf("max sub-agent total %d reached for this delegation tree", maxTotal)
		}
		k.tree[rootCorr] = n + 1
		k.treeMu.Unlock()
	}

	// Spend cap (M48): once this run's sub-agents have collectively spent past
	// SubAgentMaxSpendMicrocents, refuse further delegations — the cost analogue
	// of the fan-out count cap above. The tally is read from the journal, which
	// is durable by the time each prior child returned (bus.Publish appends
	// before it returns), so the previous delegations' spend is already visible
	// here — no in-memory accounting, race-free. Only scanned when the cap is
	// enabled, so it stays off the default path. 0 = unbounded.
	if cap := k.cfg.SubAgentMaxSpendMicrocents; cap > 0 && parentCorr != "" {
		if spent := k.subAgentSpendMicrocents(parentCorr); spent >= cap {
			return nil, fmt.Errorf("max sub-agent spend $%.4f reached", float64(cap)/1e9)
		}
	}

	childCorr := k.NewCorrelation()
	actor := "subagent-" + childCorr

	// Live-steering control surface for the sub-agent (M631): registered under
	// the child's own correlation so an operator can pause / single-step / steer
	// / resume an INDIVIDUAL sub-agent from the cockpit — reaching into the
	// delegation tree, not just the top-level lead (M608 only wired RunWith).
	// Wired into the child loop via LoopConfig.Steer below.
	rc := newRunControl()
	k.steersMu.Lock()
	k.steers[childCorr] = rc
	k.steersMu.Unlock()

	// Journal the spawn under the parent correlation so `agt why <parent>`
	// reveals the delegation and the child correlation to drill into.
	linkCorr := parentCorr
	if linkCorr == "" {
		linkCorr = childCorr
	}
	spawnPayload := map[string]any{
		"task":              task,
		"child_correlation": childCorr,
		"depth":             depth + 1,
		"parent":            parentCorr,
		"model":             subModel,
		"task_type":         taskType,
	}
	if prof != nil {
		spawnPayload["agent"] = prof.Slug // who the sub-agent ran AS (M784)
		// Delegated-wake evidence (mirrors schedule.fired / standing.fired): the
		// sub-agent's wake contract plus who delegated it and the parent run, so the
		// child run is attributable to its leader through status -> detail -> activity.
		spawnPayload["autonomy_runbook"] = roster.AutonomyRunbook(*prof)
		spawnPayload["wake_source"] = "delegated"
		if caller := strings.TrimSpace(agent.AgentFromContext(ctx)); caller != "" {
			spawnPayload["delegated_by"] = caller
		}
		if parentCorr != "" {
			spawnPayload["parent_correlation_id"] = parentCorr
		}
	}
	if async {
		spawnPayload["async"] = true // M881: non-blocking spawn; completion announced separately
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "agent." + actor + ".subagent",
		Kind:          event.KindSubAgentSpawned,
		Actor:         actor,
		CorrelationID: linkCorr,
		Payload:       spawnPayload,
	})

	// Child context: bump depth, retarget actor/correlation so the policy hook
	// and approval audit attribute the sub-agent's actions correctly.
	childCtx := delegation.WithDepth(ctx, depth+1)
	childCtx = context.WithValue(childCtx, ctxKeyActor, actor)
	childCtx = context.WithValue(childCtx, ctxKeyCorrelation, childCorr)
	// Carry the tree root to every descendant so the M629 total cap is attributed
	// to the whole tree, not re-seeded at each level.
	childCtx = context.WithValue(childCtx, ctxKeyRoot, rootCorr)
	// A named agent is a full roster identity, not only a prompt skin: carry its
	// tool policy, trust ceiling, noise policy, config overrides, lifecycle,
	// memory scope, workspace, and budget identity into the child context. The
	// explicit sub-agent LoopConfig below still owns the child system prompt,
	// selected model, and model chain so delegation-specific overrides keep their
	// existing precedence.
	if prof != nil {
		childCtx = WithAgentProfile(childCtx, *prof)
	}

	// A named agent's soul REPLACES the daemon default identity layer (it IS this
	// sub-agent's identity); the sub-agent preamble always stays on top.
	system := delegation.SystemPrompt
	switch {
	case prof != nil && agentProfileSystem(*prof) != "":
		system += "\n\n" + agentProfileSystem(*prof)
	default:
		// The LIVE default identity (SetSystem, M710) — an operator editing the
		// persona expects delegated runs to inherit the edit, not the prompt the
		// daemon booted with.
		if live := k.System(); live != "" {
			system += "\n\n" + live
		}
	}

	// The profile's per-run spend ceiling bounds this sub-agent's own run
	// (M784) — the delegation-tree spend cap above still applies on top.
	// Its ordered fallbacks become the child's model chain (M787): primary
	// first (an explicit delegate model still wins the front slot), walked
	// in order by the Governor; duplicates of the primary are skipped.
	var maxRunCost, agentDailyMc int64
	var agentSlug string
	var retryPolicy *roster.RetryPolicy
	if prof != nil {
		maxRunCost = prof.MaxCostMc
		agentSlug, agentDailyMc = prof.Slug, prof.MaxDailyMc // M793: identity ledger
		retryPolicy = prof.RetryPolicy
	}
	// modelChain + subModel were resolved (and keyed-filtered) above, before the
	// spawn was journaled.

	return &subAgentPrep{
		childCtx:     childCtx,
		childCorr:    childCorr,
		parentCorr:   parentCorr,
		rootCorr:     rootCorr,
		linkCorr:     linkCorr,
		actor:        actor,
		task:         task,
		system:       system,
		subModel:     subModel,
		modelChain:   modelChain,
		taskType:     taskType,
		maxRunCost:   maxRunCost,
		agentSlug:    agentSlug,
		agentDailyMc: agentDailyMc,
		retryPolicy:  retryPolicy,
		rc:           rc,
	}, nil
}

func (k *Kernel) validateDelegationManager(p roster.Profile, caller string) error {
	caller = strings.TrimSpace(caller)
	if caller == "" {
		return fmt.Errorf("agent %q is managed and requires a live parent/owner to delegate it", p.Slug)
	}
	manager, ok := k.roster.Get(caller)
	if !ok {
		return fmt.Errorf("agent %q manager %q is missing from the roster", p.Slug, caller)
	}
	if manager.Retired {
		return fmt.Errorf("agent %q manager %q is retired — revive it first", p.Slug, manager.Slug)
	}
	if !manager.Enabled {
		return fmt.Errorf("agent %q manager %q is paused — resume it first", p.Slug, manager.Slug)
	}
	return nil
}

// executeSubAgent runs a prepared delegation's child loop to completion and
// releases the child's own bookkeeping. Sync delegations call it inline (the
// delegate tool blocks on it); async delegations call it on a spawn goroutine.
func (k *Kernel) executeSubAgent(p *subAgentPrep) (string, error) {
	// This child may itself delegate; release its own fan-out tally and steering
	// control when it returns so the maps don't accumulate across a long-lived
	// kernel.
	defer func() {
		k.fanoutMu.Lock()
		delete(k.fanout, p.childCorr)
		k.fanoutMu.Unlock()
		k.steersMu.Lock()
		delete(k.steers, p.childCorr)
		k.steersMu.Unlock()
	}()

	var activatedSkillIDs []string
	runOnce := func() (string, []string, error) {
		// Shared governance/capacity base (LD-1): identical to RunWith's, so a
		// delegated run gets the same cost accounting (CostFn — without it the
		// per-run spend ceiling below is inert), artifact offload, context
		// compaction, and tool policy as a root run.
		lc := k.buildLoopConfig(p.childCtx, p.childCorr, p.subModel)
		system, skills, task := k.subAgentInjectedSystem(p.childCtx, p.childCorr, p.actor, p.task, p.system)
		activatedSkillIDs = delegation.AppendUniqueStrings(activatedSkillIDs, skills...)
		lc.TaskType = p.taskType     // M705: route the sub-agent (chain supplies fallbacks)
		lc.ModelChain = p.modelChain // M787: the named agent's own fallbacks win
		lc.Agent = p.agentSlug
		lc.AgentDailyCeilingMc = p.agentDailyMc
		lc.WakeSource = "subagent"
		lc.WakeReason = "delegation"
		lc.ParentCorrelation = p.parentCorr
		lc.System = system
		lc.MaxRunCostMicrocents = p.maxRunCost
		lc.Actor = p.actor
		lc.CorrelationID = p.childCorr
		lc.Steer = p.rc // M631: individual sub-agent steering
		answer, err := agent.Run(p.childCtx, lc, task)
		return answer, skills, err
	}
	answer, _, err := runOnce()
	if err != nil && p.retryPolicy != nil && p.retryPolicy.MaxAttempts > 1 {
		max := p.retryPolicy.MaxAttempts
		if max > 10 {
			max = 10
		}
		for attempt := 1; err != nil && attempt < max && agentRetryable(retryReason(err), p.retryPolicy.RetryOn); attempt++ {
			delay := retryDelay(*p.retryPolicy, attempt)
			_, _ = k.bus.Publish(event.Spec{
				Subject:       "agent." + p.agentSlug + ".retry",
				Kind:          event.KindAgentRetry,
				Actor:         "agent-retry",
				CorrelationID: p.childCorr,
				Payload: map[string]any{
					"agent":        p.agentSlug,
					"attempt":      attempt,
					"next_attempt": attempt + 1,
					"max_attempts": max,
					"reason":       retryReason(err),
					"error":        err.Error(),
					"delay_ms":     int64(delay / time.Millisecond),
					"subagent":     true,
				},
			})
			if delay > 0 {
				t := time.NewTimer(delay)
				select {
				case <-p.childCtx.Done():
					if !t.Stop() {
						select {
						case <-t.C:
						default:
						}
					}
					return "", p.childCtx.Err()
				case <-t.C:
				}
			}
			answer, _, err = runOnce()
		}
	}
	if k.forge != nil && len(activatedSkillIDs) > 0 {
		k.forge.RecordOutcome(p.childCorr, activatedSkillIDs, err == nil)
	}
	if err != nil {
		return "", fmt.Errorf("sub-agent %s: %w", p.childCorr, err)
	}
	// Day 33: lifecycle-advance lives in the runexec sub-package;
	// call through the public wrapper.
	k.CompleteAgentLifecycle(p.childCtx, p.childCorr)
	return answer, nil
}
