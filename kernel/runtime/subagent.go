// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/roster"
)

// ctxKeyDepth carries the current sub-agent nesting depth so runSubAgent can
// enforce SubAgentMaxDepth and refuse unbounded recursion. It rides the same
// context the agent loop threads into each tool Invoke.
type ctxKeyDepthT struct{}

var ctxKeyDepth = ctxKeyDepthT{}

func depthFromCtx(ctx context.Context) int {
	if v, ok := ctx.Value(ctxKeyDepth).(int); ok {
		return v
	}
	return 0
}

// subAgentSystem frames a delegated run: a focused worker that completes one
// task and reports back concisely. The kernel's own System prompt follows.
const subAgentSystem = "You are a focused sub-agent spawned to complete ONE delegated task. " +
	"Work autonomously with the tools available, then report a concise, self-contained " +
	"result the lead agent can use directly. Do not ask clarifying questions; make a " +
	"reasonable assumption and state it."

// subAgentTool is the in-process `delegate` tool (P6-MULTI-01). Its runners
// are wired to k.runSubAgent / k.runSubAgentAsync after the kernel is
// constructed (the tool is built during Open before *Kernel exists).
type subAgentTool struct {
	run   func(ctx context.Context, task, model, taskType, agentRef string) (string, error)
	spawn func(ctx context.Context, task, model, taskType, agentRef string) (string, error)
}

func newSubAgentTool() *subAgentTool { return &subAgentTool{} }

func (t *subAgentTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "delegate",
		Description: "Delegate a focused subtask to a fresh sub-agent that works " +
			"autonomously (its own tool-loop) and returns a concise result. LEAD the work: " +
			"break a big task into parts and delegate each — your sub-agents can delegate " +
			"FURTHER, so you can build a leader/worker tree, not just one flat layer. Issue " +
			"multiple delegate calls in one turn to fan out concurrently, or pass " +
			"async=true to get a spawn_id back immediately and keep working while the " +
			"sub-agent runs — collect each async result with delegate_await BEFORE you " +
			"give your final answer (un-awaited sub-agents are cancelled when your run " +
			"ends). Prefer reusing an existing named `agent` (roster slug) whose role fits " +
			"over inventing an ad-hoc one. Optionally pick the sub-agent's model (otherwise " +
			"the daemon default) and/or its routing task type (defaults to \"delegate\"); a " +
			"configured routing chain for that task type provides the fallback models.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "task": {
      "type": "string",
      "description": "The complete, self-contained instruction for the sub-agent. Include all context it needs; it does not see this conversation."
    },
    "model": {
      "type": "string",
      "description": "Optional model id for the sub-agent (e.g. a cheaper or stronger model than the lead). Omit to use the daemon default."
    },
    "task_type": {
      "type": "string",
      "description": "Optional routing task type for the sub-agent (e.g. \"code\", \"plan\"); its configured model chain supplies the fallbacks. Defaults to \"delegate\"."
    },
    "agent": {
      "type": "string",
      "description": "Optional named agent (roster slug) to run the sub-agent AS: its soul becomes the sub-agent's persona and its model/task type/cost ceiling apply as defaults. Explicit model/task_type here still win."
    },
    "async": {
      "type": "boolean",
      "description": "When true, return immediately with a spawn_id while the sub-agent runs in the background; collect its result later with delegate_await. Default false (wait for the result)."
    }
  },
  "required": ["task"]
}`),
	}
}

func (t *subAgentTool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	var in struct {
		Task     string `json:"task"`
		Model    string `json:"model"`
		TaskType string `json:"task_type"`
		Agent    string `json:"agent"`
		Async    bool   `json:"async"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid input: " + err.Error(), IsError: true}, nil
	}
	if in.Async {
		if t.spawn == nil {
			return agent.Result{Output: "sub-agent spawner not wired", IsError: true}, nil
		}
		id, err := t.spawn(ctx, in.Task, in.Model, in.TaskType, in.Agent)
		if err != nil {
			return agent.Result{Output: "delegation failed: " + err.Error(), IsError: true}, nil
		}
		return agent.Result{Output: fmt.Sprintf("spawned sub-agent %s — it is working in the background. Collect its result with delegate_await {\"spawn_id\":%q} before your final answer.", id, id)}, nil
	}
	if t.run == nil {
		return agent.Result{Output: "sub-agent runner not wired", IsError: true}, nil
	}
	out, err := t.run(ctx, in.Task, in.Model, in.TaskType, in.Agent)
	if err != nil {
		// Surface as a tool error so the lead agent can adapt, not crash.
		return agent.Result{Output: "delegation failed: " + err.Error(), IsError: true}, nil
	}
	return agent.Result{Output: out}, nil
}

// subAgentAwaitTool is the in-process `delegate_await` tool (M881): the
// collect half of async delegation. Its runner is wired to k.awaitSubAgent
// after the kernel is constructed.
type subAgentAwaitTool struct {
	await func(ctx context.Context, spawnID string) (agent.Result, error)
}

func newSubAgentAwaitTool() *subAgentAwaitTool { return &subAgentAwaitTool{} }

func (t *subAgentAwaitTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "delegate_await",
		Description: "Wait for an async delegation (delegate with async=true) to finish and " +
			"return its result. Call it once per spawn_id; issue several delegate_await calls " +
			"in one turn to collect a whole fan-out. If it reports the sub-agent is still " +
			"running, call it again. A result can be collected exactly once.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "spawn_id": {
      "type": "string",
      "description": "The spawn_id returned by delegate(async=true)."
    }
  },
  "required": ["spawn_id"]
}`),
	}
}

func (t *subAgentAwaitTool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	var in struct {
		SpawnID string `json:"spawn_id"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid input: " + err.Error(), IsError: true}, nil
	}
	if t.await == nil {
		return agent.Result{Output: "sub-agent awaiter not wired", IsError: true}, nil
	}
	return t.await(ctx, in.SpawnID)
}

// spawnHandle tracks one asynchronously delegated sub-agent (M881): the child
// runs on its own goroutine while the lead keeps working; the lead collects
// the result via delegate_await. Guarded by k.mu except done (close-once
// signal) and the result fields, which are written exactly once before done
// is closed and read only after it is.
type spawnHandle struct {
	parentCorr string
	rootCorr   string
	cancel     context.CancelFunc
	done       chan struct{}
	answer     string
	err        error
}

// subAgentPrep carries a fully resolved + journaled delegation, ready to
// execute: prepareSubAgent applied every guard (depth, fan-out, tree total,
// spend), resolved the model/persona, registered steering, and published
// subagent.spawned; executeSubAgent runs the child loop and cleans up.
type subAgentPrep struct {
	childCtx     context.Context
	childCorr    string
	parentCorr   string
	rootCorr     string
	linkCorr     string
	actor        string
	task         string
	system       string
	subModel     string
	modelChain   []string
	taskType     string
	maxRunCost   int64
	agentSlug    string
	agentDailyMc int64
	rc           *runControl
}

// runSubAgent executes a delegated task as a nested agent.Run under a fresh
// child correlation, bounded by SubAgentMaxDepth. The spawn is journaled under
// the PARENT correlation (carrying the child correlation) so `agt why <parent>`
// shows the delegation; the child's own steps live under the child correlation.
func (k *Kernel) runSubAgent(ctx context.Context, task, model, taskType, agentRef string) (string, error) {
	p, err := k.prepareSubAgent(ctx, task, model, taskType, agentRef, false)
	if err != nil {
		return "", err
	}
	return k.executeSubAgent(p)
}

// runSubAgentAsync is the non-blocking spawn half of async delegation (M881):
// it applies the exact same guards and journaling as a synchronous delegate,
// then runs the child on its own goroutine and returns its spawn id (the
// child correlation) immediately. Completion is announced push-style as a
// subagent.completed event under the parent correlation, and the result is
// collected via delegate_await. The child's lifetime is detached from the
// spawning TOOL CALL (whose context ends when delegate returns) but stays
// bounded by the kernel: its cancel is registered in k.runs (so Halt and
// CancelRun reach it) and the parent run's cleanup cancels any un-awaited
// children, so a spawn never outlives its delegation tree.
func (k *Kernel) runSubAgentAsync(ctx context.Context, task, model, taskType, agentRef string) (string, error) {
	p, err := k.prepareSubAgent(ctx, task, model, taskType, agentRef, true)
	if err != nil {
		return "", err
	}
	// WithoutCancel keeps the run-stamped values (depth, root, actor, child
	// correlation, memory scope, workdir) while dropping the tool-call
	// deadline/cancel; the fresh WithCancel re-attaches a kill switch owned
	// by the kernel instead of the spawning call.
	childCtx, cancel := context.WithCancel(context.WithoutCancel(p.childCtx))
	p.childCtx = childCtx
	h := &spawnHandle{
		parentCorr: p.parentCorr,
		rootCorr:   p.rootCorr,
		cancel:     cancel,
		done:       make(chan struct{}),
	}
	k.mu.Lock()
	if k.halted {
		// Halt won the race after prepare's check: don't start a goroutine
		// Halt's sweep of k.runs can no longer see.
		k.mu.Unlock()
		cancel()
		return "", ErrHalted
	}
	k.spawns[p.childCorr] = h
	k.runs[p.childCorr] = cancel
	k.runWG.Add(1) // Close drains async spawns like any in-flight run (M883)
	k.mu.Unlock()
	go func() {
		defer k.runWG.Done()
		defer cancel()
		answer, err := k.executeSubAgent(p)
		h.answer, h.err = answer, err
		k.mu.Lock()
		delete(k.runs, p.childCorr)
		k.mu.Unlock()
		// Announce completion under the parent correlation BEFORE releasing
		// awaiters, so by the time delegate_await returns the outcome is
		// already durably journaled (subscribable by the UI as a push signal).
		payload := map[string]any{
			"child_correlation": p.childCorr,
			"ok":                err == nil,
			"async":             true,
		}
		if err != nil {
			payload["error"] = err.Error()
		} else {
			payload["chars"] = len(answer)
		}
		_, _ = k.bus.Publish(event.Spec{
			Subject:       "agent." + p.actor + ".subagent",
			Kind:          event.KindSubAgentCompleted,
			Actor:         p.actor,
			CorrelationID: p.linkCorr,
			Payload:       payload,
		})
		close(h.done)
	}()
	return p.childCorr, nil
}

// awaitSubAgent blocks until an async delegation finishes (or the calling
// tool context ends) and returns its result exactly once (M881). Only the
// spawning run may collect — a foreign correlation asking for someone else's
// spawn id is refused.
func (k *Kernel) awaitSubAgent(ctx context.Context, spawnID string) (agent.Result, error) {
	spawnID = strings.TrimSpace(spawnID)
	if spawnID == "" {
		return agent.Result{Output: "spawn_id required", IsError: true}, nil
	}
	k.mu.Lock()
	h, ok := k.spawns[spawnID]
	k.mu.Unlock()
	if !ok {
		return agent.Result{Output: fmt.Sprintf("unknown spawn id %q (already collected, cancelled, or never spawned)", spawnID), IsError: true}, nil
	}
	if caller := correlationFromCtx(ctx); caller != "" && caller != h.parentCorr {
		return agent.Result{Output: fmt.Sprintf("spawn %s belongs to another run", spawnID), IsError: true}, nil
	}
	select {
	case <-ctx.Done():
		// The per-tool timeout (or a run-level cancel) fired while the child
		// is still working. The handle stays collectable: the model can call
		// delegate_await again; a genuine run cancel ends the loop upstream.
		return agent.Result{Output: fmt.Sprintf("sub-agent %s is still running — call delegate_await again to keep waiting", spawnID), IsError: true}, nil
	case <-h.done:
	}
	k.mu.Lock()
	delete(k.spawns, spawnID)
	k.mu.Unlock()
	if h.err != nil {
		return agent.Result{Output: "delegation failed: " + h.err.Error(), IsError: true}, nil
	}
	return agent.Result{Output: h.answer}, nil
}

// prepareSubAgent resolves and journals one delegation: every bound (depth,
// fan-out, tree total, spend) is enforced HERE, at spawn time, identically
// for sync and async paths, and the subagent.spawned event is published
// before the caller decides how to execute.
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
		if taskType == "" {
			taskType = strings.TrimSpace(prof.TaskType)
		}
	}
	if taskType == "" {
		taskType = "delegate"
	}
	subModel := k.cfg.Model
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
		subModel, modelChain = keyedModelChain(subModel, modelChain, avail, k.cfg.Model)
	}

	depth := depthFromCtx(ctx)
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
		k.mu.Lock()
		n := k.fanout[parentCorr]
		if n >= maxFanout {
			k.mu.Unlock()
			return nil, fmt.Errorf("max sub-agent fan-out %d reached", maxFanout)
		}
		k.fanout[parentCorr] = n + 1
		k.mu.Unlock()
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
		k.mu.Lock()
		n := k.tree[rootCorr]
		if n >= maxTotal {
			k.mu.Unlock()
			return nil, fmt.Errorf("max sub-agent total %d reached for this delegation tree", maxTotal)
		}
		k.tree[rootCorr] = n + 1
		k.mu.Unlock()
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
	k.mu.Lock()
	k.steers[childCorr] = rc
	k.mu.Unlock()

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
	childCtx := context.WithValue(ctx, ctxKeyDepth, depth+1)
	childCtx = context.WithValue(childCtx, ctxKeyActor, actor)
	childCtx = context.WithValue(childCtx, ctxKeyCorrelation, childCorr)
	// Carry the tree root to every descendant so the M629 total cap is attributed
	// to the whole tree, not re-seeded at each level.
	childCtx = context.WithValue(childCtx, ctxKeyRoot, rootCorr)
	// A named agent's memory follows it (M786): the child's memory-tool recalls
	// default to the profile's scope (its private notes + shared memory). Its
	// working directory follows too (M792): the child's file/shell tools
	// operate inside the profile's workspace subdirectory.
	if prof != nil {
		scope := strings.TrimSpace(prof.MemoryScope)
		if scope == "" {
			scope = prof.Slug
		}
		childCtx = memory.WithScope(childCtx, scope)
		childCtx = agent.WithWorkdir(childCtx, prof.Workdir)
	}

	// A named agent's soul REPLACES the daemon persona layer (it IS this
	// sub-agent's identity); the sub-agent preamble always stays on top.
	system := subAgentSystem
	switch {
	case prof != nil && strings.TrimSpace(prof.Soul) != "":
		system += "\n\n" + strings.TrimSpace(prof.Soul)
	case k.cfg.System != "":
		system += "\n\n" + k.cfg.System
	}

	// The profile's per-run spend ceiling bounds this sub-agent's own run
	// (M784) — the delegation-tree spend cap above still applies on top.
	// Its ordered fallbacks become the child's model chain (M787): primary
	// first (an explicit delegate model still wins the front slot), walked
	// in order by the Governor; duplicates of the primary are skipped.
	var maxRunCost, agentDailyMc int64
	var agentSlug string
	if prof != nil {
		maxRunCost = prof.MaxCostMc
		agentSlug, agentDailyMc = prof.Slug, prof.MaxDailyMc // M793: identity ledger
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
		rc:           rc,
	}, nil
}

// executeSubAgent runs a prepared delegation's child loop to completion and
// releases the child's own bookkeeping. Sync delegations call it inline (the
// delegate tool blocks on it); async delegations call it on a spawn goroutine.
func (k *Kernel) executeSubAgent(p *subAgentPrep) (string, error) {
	// This child may itself delegate; release its own fan-out tally and steering
	// control when it returns so the maps don't accumulate across a long-lived
	// kernel.
	defer func() {
		k.mu.Lock()
		delete(k.fanout, p.childCorr)
		delete(k.steers, p.childCorr)
		k.mu.Unlock()
	}()

	answer, err := agent.Run(p.childCtx, agent.LoopConfig{
		Provider:             k.cfg.Provider,
		Tools:                k.mergeMCPTools(k.mergeScriptTools(k.tools)), // forged + MCP tools reach sub-agents too (M794/M796)
		Bus:                  k.bus,
		Model:                p.subModel,
		TaskType:             p.taskType,   // M705: route the sub-agent (chain supplies fallbacks)
		ModelChain:           p.modelChain, // M787: the named agent's own fallbacks win
		Agent:                p.agentSlug,
		AgentDailyCeilingMc:  p.agentDailyMc,
		System:               p.system,
		MaxIter:              k.cfg.MaxIter,
		MaxAutoContinue:      k.cfg.MaxAutoContinue,  // M833: autonomous continue past MaxIter
		AutoContinueWait:     k.cfg.AutoContinueWait, // M833
		ToolTimeout:          k.cfg.ToolTimeout,
		MaxParallelTools:     k.cfg.MaxParallelTools, // M880: in-turn parallel tool dispatch
		MaxRunCostMicrocents: p.maxRunCost,
		Actor:                p.actor,
		CorrelationID:        p.childCorr,
		Policy:               k.policyHook,
		Steer:                p.rc, // M631: individual sub-agent steering
	}, p.task)
	if err != nil {
		return "", fmt.Errorf("sub-agent %s: %w", p.childCorr, err)
	}
	return answer, nil
}

// subAgentSpendMicrocents sums the spend (budget.consumed cost_microcents, M47)
// of every run descended from parentCorr — its sub-agents and their sub-agents,
// transitively — excluding parentCorr's own direct spend. It backs the M48
// spend cap: a single forward journal pass builds the parent→children links
// (from subagent.spawned) and the per-run spend (from budget.consumed), then
// totals the spend over parentCorr's transitive descendants. Stateless and
// race-free: every prior delegation's spend is already durably journaled by the
// time the next delegate calls this. Only invoked when the cap is enabled.
func (k *Kernel) subAgentSpendMicrocents(parentCorr string) int64 {
	childrenOf := map[string][]string{}
	spendOf := map[string]int64{}
	_ = k.journal.Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindSubAgentSpawned:
			if child, parent := spawnLink(e.Payload); child != "" && parent != "" {
				childrenOf[parent] = append(childrenOf[parent], child)
			}
		case event.KindBudgetConsumed:
			if e.CorrelationID != "" {
				spendOf[e.CorrelationID] += budgetCostMicrocents(e.Payload)
			}
		}
		return nil
	})

	// Sum spend over the transitive descendants of parentCorr (BFS over the
	// links), excluding parentCorr itself — the cap bounds sub-agent spend, not
	// the lead's own. A `seen` set guards against a malformed cyclic link.
	var total int64
	seen := map[string]bool{parentCorr: true}
	queue := append([]string{}, childrenOf[parentCorr]...)
	for len(queue) > 0 {
		corr := queue[0]
		queue = queue[1:]
		if seen[corr] {
			continue
		}
		seen[corr] = true
		total += spendOf[corr]
		queue = append(queue, childrenOf[corr]...)
	}
	return total
}

// spawnLink pulls child + parent correlation ids out of a subagent.spawned
// payload (M48 mirror of the control plane's extractSpawnLink). Returns
// ("","") on parse failure so an unparseable link is simply skipped.
func spawnLink(payload json.RawMessage) (child, parent string) {
	if len(payload) == 0 {
		return "", ""
	}
	var p struct {
		Child  string `json:"child_correlation"`
		Parent string `json:"parent"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", ""
	}
	return p.Child, p.Parent
}

// budgetCostMicrocents pulls cost_microcents out of a budget.consumed payload
// (M48). Returns 0 on parse failure so an unparseable spend event contributes
// nothing. JSON numbers decode as float64; the integer microcents have no
// fractional part to lose.
func budgetCostMicrocents(payload json.RawMessage) int64 {
	if len(payload) == 0 {
		return 0
	}
	var p struct {
		Cost float64 `json:"cost_microcents"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return 0
	}
	return int64(p.Cost)
}

// keyedModelChain restricts a delegation's effective model chain to models a
// KEYED provider can serve (M838 bugfix). It folds subModel + the profile chain
// into one de-duped ordered list, keeps only those for which avail() is true,
// and — if nothing keyed survives — falls back to def (the daemon's active,
// keyed model). Returns the chosen primary model and the chain (nil when a single
// model, matching the pre-filter convention). A nil/empty result leaves the
// caller's values unchanged.
func keyedModelChain(subModel string, modelChain []string, avail func(string) bool, def string) (string, []string) {
	chain := []string{}
	if subModel != "" {
		chain = append(chain, subModel)
	}
	for _, m := range modelChain {
		if m != "" && !slices.Contains(chain, m) {
			chain = append(chain, m)
		}
	}
	kept := make([]string, 0, len(chain))
	for _, m := range chain {
		if avail(m) {
			kept = append(kept, m)
		}
	}
	if len(kept) == 0 {
		if d := strings.TrimSpace(def); d != "" {
			kept = append(kept, d)
		}
	}
	if len(kept) == 0 {
		return subModel, modelChain // nothing to do; keep originals
	}
	if len(kept) == 1 {
		return kept[0], nil
	}
	return kept[0], kept
}
