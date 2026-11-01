// SPDX-License-Identifier: MIT

package delegation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
	"github.com/agezt/agezt/kernel/modules/runs/steer"
)

// prepare resolves and journals one delegation: every bound (depth, fan-out,
// tree total, spend) is enforced HERE, at spawn time, identically for the
// sync and the async path, and the subagent.spawned event is published before
// the caller decides how to execute.
func (s *Service) prepare(ctx context.Context, req Request, async bool) (*Plan, error) {
	task := strings.TrimSpace(req.Task)
	if task == "" {
		return nil, errors.New("task required")
	}
	model := strings.TrimSpace(req.Model)
	taskType := strings.TrimSpace(req.TaskType)
	agentRef := strings.TrimSpace(req.AgentRef)

	// Delegate AS a named agent (M784): resolve the roster profile up front so
	// an unknown or paused agent is a clear tool error the lead adapts to, not
	// a sub-agent silently running as the default identity. The profile's
	// model/task type/cost ceiling apply as DEFAULTS; explicit call args win.
	var prof *agentapi.Profile
	if agentRef != "" {
		p, ok := s.host.RosterGet(agentRef)
		if !ok {
			return nil, fmt.Errorf("unknown agent %q (agt agent list)", agentRef)
		}
		if p.Retired {
			return nil, fmt.Errorf("agent %q is retired — revive it first (agt agent revive %s)", p.Slug, p.Slug)
		}
		if !p.Enabled {
			return nil, fmt.Errorf("agent %q is paused (agt agent resume %s)", p.Slug, p.Slug)
		}
		caller := toolapi.AgentFromContext(ctx)
		if !p.AllowsDelegationFrom(caller) {
			manager := strings.TrimSpace(p.ParentAgent)
			if manager == "" {
				manager = strings.TrimSpace(p.OwnerAgent)
			}
			return nil, fmt.Errorf("agent %q is managed by %q and cannot be delegated by %q", p.Slug, manager, caller)
		}
		if !p.AllowsDirectCall() {
			if err := s.validateManager(p, caller); err != nil {
				return nil, err
			}
		}
		prof = &p
	}
	// Per-sub-agent model (M705): an explicit model overrides the daemon default
	// for this delegation; an explicit task_type selects the routing chain whose
	// models provide the fallbacks (defaulting to "delegate"). Both are optional —
	// a bare delegate behaves exactly as before.
	if prof != nil {
		if model == "" {
			// The profile's own model, or its AGEZT_MODEL config override
			// when it has none. The daemon reads the override: the
			// override vocabulary is its own configuration's.
			model = strings.TrimSpace(s.host.ProfileModel(*prof))
		}
		if taskType == "" {
			taskType = strings.TrimSpace(prof.TaskType)
		}
	}
	if taskType == "" {
		taskType = "delegate"
	}
	// The live daemon default (the daemon's Model(), hot-swapped on provider
	// reload — M816), not the boot seed: a delegation falling back to the boot
	// model would keep asking for whatever the daemon started with, which after
	// a key rotation is typically the model that just stopped being servable.
	defaultModel := s.host.Model()
	subModel := defaultModel
	if model != "" {
		subModel = model
	}
	if s.host.Halted() {
		return nil, s.host.HaltedError()
	}

	cfg := s.config()

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
	if avail := cfg.ModelAvailable; avail != nil {
		subModel, modelChain = KeyedModelChain(subModel, modelChain, avail, defaultModel)
	}

	depth := DepthFromCtx(ctx)
	maxDepth := cfg.maxDepth()
	if depth >= maxDepth {
		return nil, fmt.Errorf("max sub-agent depth %d reached", maxDepth)
	}

	parentCorr := runctx.CorrelationFromCtx(ctx)

	// Fan-out bound (M46): depth caps how DEEP delegation nests; this caps how
	// WIDE a single run fans out. We tally sub-agents per spawning correlation
	// (the lead run, or a sub-agent that itself delegates) and refuse the Nth+1
	// call with a tool error the lead adapts to. 0 = unbounded (default). A
	// correlation-less spawn (no run context) can't be attributed, so it's left
	// unbounded. The tally is released when the spawning run ends (the daemon's
	// cleanup for a top-level run; this function's release for a nested
	// spawner's own child correlation below).
	if maxFanout := cfg.MaxFanout; maxFanout > 0 && parentCorr != "" {
		s.mu.Lock()
		n := s.fanout[parentCorr]
		if n >= maxFanout {
			s.mu.Unlock()
			return nil, fmt.Errorf("max sub-agent fan-out %d reached", maxFanout)
		}
		s.fanout[parentCorr] = n + 1
		s.mu.Unlock()
	}

	// Tree-total bound (M629): depth caps how DEEP and fan-out caps how WIDE at
	// one level, but a depth-D, fan-out-F tree can still hold up to F^D agents —
	// neither cap bounds the WHOLE tree's size. This caps the total sub-agents
	// across every depth of one delegation tree, attributed to the root run's
	// correlation. The root is the top-level lead (runctx.RootFromCtx is empty at depth
	// 0, so the lead's own correlation seeds the root); every descendant inherits
	// it via childCtx below, so a spawn three levels down still counts against
	// the same root. 0 = unbounded. The tally is released when the root run ends
	// (the daemon's cleanup path deletes the root's tally).
	rootCorr := runctx.RootFromCtx(ctx)
	if rootCorr == "" {
		rootCorr = parentCorr // this spawner is the tree root
	}
	if maxTotal := cfg.MaxTotal; maxTotal > 0 && rootCorr != "" {
		s.mu.Lock()
		n := s.tree[rootCorr]
		if n >= maxTotal {
			s.mu.Unlock()
			return nil, fmt.Errorf("max sub-agent total %d reached for this delegation tree", maxTotal)
		}
		s.tree[rootCorr] = n + 1
		s.mu.Unlock()
	}

	// Spend cap (M48): once this run's sub-agents have collectively spent past
	// MaxSpendMicrocents, refuse further delegations — the cost analogue of the
	// fan-out count cap above. The tally is read from the journal, which is
	// durable by the time each prior child returned (bus.Publish appends
	// before it returns), so the previous delegations' spend is already visible
	// here — no in-memory accounting, race-free. Only scanned when the cap is
	// enabled, so it stays off the default path. 0 = unbounded.
	if cap := cfg.MaxSpendMicrocents; cap > 0 && parentCorr != "" {
		if spent := s.spendMicrocents(parentCorr); spent >= cap {
			return nil, fmt.Errorf("max sub-agent spend $%.4f reached", float64(cap)/1e9)
		}
	}

	childCorr := s.host.NewCorrelation()
	actor := "subagent-" + childCorr

	// Live-steering control surface for the sub-agent (M631): registered under
	// the child's own correlation so an operator can pause / single-step / steer
	// / resume an INDIVIDUAL sub-agent from the cockpit — reaching into the
	// delegation tree, not just the top-level lead (M608 only wired RunWith).
	// Wired into the child loop via LoopConfig.Steer below.
	rc := steer.NewControl()
	s.host.RegisterSteer(childCorr, rc)

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
		spawnPayload["autonomy_runbook"] = agentapi.AutonomyRunbook(*prof)
		spawnPayload["wake_source"] = "delegated"
		if caller := strings.TrimSpace(toolapi.AgentFromContext(ctx)); caller != "" {
			spawnPayload["delegated_by"] = caller
		}
		if parentCorr != "" {
			spawnPayload["parent_correlation_id"] = parentCorr
		}
	}
	if async {
		spawnPayload["async"] = true // M881: non-blocking spawn; completion announced separately
	}
	_, _ = s.host.Publish(event.Spec{
		Subject:       "agent." + actor + ".subagent",
		Kind:          event.KindSubAgentSpawned,
		Actor:         actor,
		CorrelationID: linkCorr,
		Payload:       spawnPayload,
	})

	// Child context: bump depth, retarget actor/correlation so the policy hook
	// and approval audit attribute the sub-agent's actions correctly.
	childCtx := WithDepth(ctx, depth+1)
	childCtx = runctx.WithActor(childCtx, actor, childCorr)
	// Carry the tree root to every descendant so the M629 total cap is attributed
	// to the whole tree, not re-seeded at each level.
	childCtx = runctx.WithRoot(childCtx, rootCorr)
	// A named agent is a full roster identity, not only a prompt skin: carry its
	// tool policy, trust ceiling, noise policy, config overrides, lifecycle,
	// memory scope, workspace, and budget identity into the child context. The
	// explicit sub-agent LoopConfig below still owns the child system prompt,
	// selected model, and model chain so delegation-specific overrides keep their
	// existing precedence.
	if prof != nil {
		childCtx = runctx.WithAgentProfile(childCtx, *prof)
	}

	// A named agent's soul REPLACES the daemon default identity layer (it IS this
	// sub-agent's identity); the sub-agent preamble always stays on top.
	system := SystemPrompt
	switch {
	case prof != nil && runctx.AgentProfileSystem(*prof) != "":
		system += "\n\n" + runctx.AgentProfileSystem(*prof)
	default:
		// The LIVE default identity (SetSystem, M710) — an operator editing the
		// persona expects delegated runs to inherit the edit, not the prompt the
		// daemon booted with.
		if live := s.host.System(); live != "" {
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
	var retryPolicy *agentapi.RetryPolicy
	if prof != nil {
		maxRunCost = prof.MaxCostMc
		agentSlug, agentDailyMc = prof.Slug, prof.MaxDailyMc // M793: identity ledger
		retryPolicy = prof.RetryPolicy
	}
	// modelChain + subModel were resolved (and keyed-filtered) above, before the
	// spawn was journaled.

	return &Plan{
		ChildCtx:     childCtx,
		ChildCorr:    childCorr,
		ParentCorr:   parentCorr,
		RootCorr:     rootCorr,
		LinkCorr:     linkCorr,
		Actor:        actor,
		Task:         task,
		System:       system,
		SubModel:     subModel,
		ModelChain:   modelChain,
		TaskType:     taskType,
		MaxRunCost:   maxRunCost,
		AgentSlug:    agentSlug,
		AgentDailyMc: agentDailyMc,
		RetryPolicy:  retryPolicy,
		Steer:        rc,
	}, nil
}

// validateManager checks that a managed agent's manager is live enough to
// delegate it: the manager must exist, not be retired and not be paused.
func (s *Service) validateManager(p agentapi.Profile, caller string) error {
	caller = strings.TrimSpace(caller)
	if caller == "" {
		return fmt.Errorf("agent %q is managed and requires a live parent/owner to delegate it", p.Slug)
	}
	manager, ok := s.host.RosterGet(caller)
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

// config returns the live delegation policy.
func (s *Service) config() Config {
	if s.deps.Config == nil {
		return Config{}
	}
	return s.deps.Config()
}
