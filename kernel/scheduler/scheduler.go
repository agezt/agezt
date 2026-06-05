// SPDX-License-Identifier: MIT

// Package scheduler is the DAG layer that sits *above* the first-party
// single-agent tool-loop (DECISIONS B0d, SPEC-02 §4). A Plan is a
// directed-acyclic graph of named Nodes; the Executor walks the graph
// topologically, runs independent branches in parallel under a bounded
// worker pool, and publishes node.* + plan.* events on the bus.
//
// Two Node types ship in M1.e:
//
//   - LoopNode  — wraps one agent.Run; the existing tool-loop becomes
//     a single node, so a 1-node plan is identical to
//     today's `agt run` end-to-end.
//   - GateNode  — synchronously submits an approval.Request and
//     blocks until the operator decides (or the configured
//     timeout). A deny aborts the plan; a grant releases
//     the downstream branch.
//
// Future node types (per SPEC-02 §4.2): llm, tool, agent (parallel
// sub-agent spawn), coding. The Node interface is intentionally narrow
// so they slot in without changing the executor.
//
// Determinism: given a fixed plan + journal prefix, re-execution is
// reproducible up to LLM nondeterminism (SPEC-02 §4.4 — exactly the
// same guarantee the bare tool-loop already gives). The scheduler
// adds nothing stochastic of its own.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"maps"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

// DefaultMaxParallel is the bounded worker pool size when Plan.MaxParallel
// is zero (SPEC-02 §4.3 default).
const DefaultMaxParallel = 8

// NodeKind names the canonical node types (SPEC-02 §4.2). Future kinds
// are appended; never renumbered or renamed.
type NodeKind string

const (
	KindLoop NodeKind = "loop"
	KindGate NodeKind = "gate"
)

// Node is one unit of work in a Plan.
type Node interface {
	// ID is the per-plan stable identifier ("research", "approve",
	// "execute"). Must be unique within the Plan; used to express
	// dependencies and to subject node.* events.
	ID() string
	// Kind reports the node's type for events + UIs.
	Kind() NodeKind
	// DependsOn returns the IDs of nodes that must complete
	// successfully before this one starts. Empty = roots.
	DependsOn() []string
	// Run executes the node and returns its result. The supplied
	// Inputs carries the Result of each upstream dependency keyed by
	// dependency ID. ctx is the per-plan ctx; cancellation halts the
	// node.
	Run(ctx context.Context, in Inputs) (Result, error)
}

// Inputs is a snapshot of every completed upstream dependency's
// Result, keyed by node ID.
type Inputs map[string]Result

// Result is what a Node produced. The Output is opaque to the executor
// — downstream nodes inspect it via the Inputs map. The string form is
// what surfaces in the node.completed event payload.
type Result struct {
	// Output is the node's primary product. For LoopNode this is the
	// final answer string; for GateNode this is the grant reason.
	Output string
	// Detail is structured metadata for the event payload; nodes that
	// want richer per-event detail (token counts, decisions) put it
	// here. May be nil.
	Detail map[string]any
}

// Plan is a directed-acyclic graph of Nodes.
type Plan struct {
	// Name is a human label that lands in the plan.* event payloads
	// (e.g. "research-then-execute"). Optional.
	Name string
	// Nodes is the full set of nodes. Order doesn't matter; the
	// executor topologically sorts them.
	Nodes []Node
	// MaxParallel caps how many nodes may run concurrently. 0 → use
	// DefaultMaxParallel.
	MaxParallel int
}

// PlanResult summarises an end-to-end Plan run. NodeResults is keyed
// by Node ID; Errors contains every node that failed (Plan can fail
// on the first error, or surface all of them — see Executor.Stop).
type PlanResult struct {
	PlanID      string
	NodeResults map[string]Result
	Errors      map[string]error
}

// Executor runs a Plan. One Executor instance per kernel; safe for
// concurrent Run calls (each plan gets its own correlation_id).
type Executor struct {
	bus *bus.Bus
	now func() time.Time
}

// Config configures an Executor.
type Config struct {
	Bus *bus.Bus
	Now func() time.Time
}

// New constructs an Executor.
func New(cfg Config) *Executor {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Executor{bus: cfg.Bus, now: now}
}

// ErrCycle is returned by Run when the supplied Plan contains a cycle.
var ErrCycle = errors.New("scheduler: plan has a cycle")

// ErrDuplicateNodeID is returned when the same ID appears on two Nodes.
var ErrDuplicateNodeID = errors.New("scheduler: duplicate node ID")

// ErrUnknownDependency is returned when a Node depends on an ID that
// no other Node in the Plan provides.
var ErrUnknownDependency = errors.New("scheduler: unknown dependency")

// ErrEmptyPlan is returned for a Plan with no Nodes.
var ErrEmptyPlan = errors.New("scheduler: plan has no nodes")

// Run executes the plan to completion or the first node failure.
// CorrelationID, if empty, is generated.
func (e *Executor) Run(ctx context.Context, plan Plan, correlationID string) (*PlanResult, error) {
	if len(plan.Nodes) == 0 {
		return nil, ErrEmptyPlan
	}
	if correlationID == "" {
		correlationID = "plan-" + ulid.New()
	}
	maxParallel := plan.MaxParallel
	if maxParallel <= 0 {
		maxParallel = DefaultMaxParallel
	}

	// Index nodes by ID; reject duplicates and unknown dependencies.
	byID := make(map[string]Node, len(plan.Nodes))
	for _, n := range plan.Nodes {
		if _, dup := byID[n.ID()]; dup {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateNodeID, n.ID())
		}
		byID[n.ID()] = n
	}
	for _, n := range plan.Nodes {
		for _, dep := range n.DependsOn() {
			if _, ok := byID[dep]; !ok {
				return nil, fmt.Errorf("%w: %q -> %q", ErrUnknownDependency, n.ID(), dep)
			}
		}
	}

	// Verify acyclicity via Kahn's algorithm. We don't need the order
	// itself (the executor schedules dynamically on completion) but
	// the absence of a cycle is mandatory.
	if err := assertAcyclic(plan.Nodes); err != nil {
		return nil, err
	}

	planID := correlationID
	e.publishPlanStarted(planID, plan)

	// State for the dynamic scheduler.
	var (
		mu        sync.Mutex
		results   = map[string]Result{}
		errs      = map[string]error{}
		completed = map[string]struct{}{}
		started   = map[string]struct{}{}
	)

	// indegree counts how many unmet deps each node has.
	indegree := make(map[string]int, len(plan.Nodes))
	for _, n := range plan.Nodes {
		indegree[n.ID()] = len(n.DependsOn())
	}

	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	runCtx = withCorrelation(runCtx, planID)

	// readyToRun returns IDs that have indegree 0 and have not started
	// yet, and that have no failed dependency.
	pickReady := func() []string {
		mu.Lock()
		defer mu.Unlock()
		var ready []string
		for id, deg := range indegree {
			if deg != 0 {
				continue
			}
			if _, run := started[id]; run {
				continue
			}
			// Skip if any upstream errored (we treat one error as a
			// terminal stop; M1.e doesn't ship compensation paths).
			ok := true
			for _, dep := range byID[id].DependsOn() {
				if _, failed := errs[dep]; failed {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			started[id] = struct{}{}
			ready = append(ready, id)
		}
		// Stable ordering helps tests + per-tick determinism.
		sort.Strings(ready)
		return ready
	}

	runNode := func(id string, holdsSlot bool) {
		defer wg.Done()
		// Gate nodes block on a HUMAN decision, not on compute, so they must not
		// occupy a worker-pool slot: otherwise a gate awaiting approval would
		// starve unrelated ready nodes (with MaxParallel low, a single pending gate
		// could stall the whole frontier for the entire approval window). Only
		// compute nodes are bounded by the semaphore. Acquiring it here (inside the
		// goroutine) rather than in the driver also means launching a node never
		// blocks the driver, so a gate listed after a slot-bound compute node still
		// starts immediately.
		if holdsSlot {
			sem <- struct{}{}
			defer func() { <-sem }()
		}

		node := byID[id]
		mu.Lock()
		// Build per-node Inputs from completed upstreams.
		inputs := make(Inputs, len(node.DependsOn()))
		for _, dep := range node.DependsOn() {
			if r, ok := results[dep]; ok {
				inputs[dep] = r
			}
		}
		mu.Unlock()

		e.publishNodeStarted(planID, node)

		res, err := node.Run(runCtx, inputs)

		mu.Lock()
		completed[id] = struct{}{}
		if err != nil {
			errs[id] = err
		} else {
			results[id] = res
		}
		// Decrement downstream indegrees regardless — failed branches
		// are pruned via the failed-upstream check in pickReady.
		for _, m := range plan.Nodes {
			for _, dep := range m.DependsOn() {
				if dep == id {
					indegree[m.ID()]--
				}
			}
		}
		mu.Unlock()

		if err != nil {
			e.publishNodeFailed(planID, node, err)
		} else {
			e.publishNodeCompleted(planID, node, res)
		}
	}

	// Drive the scheduler: launch every newly-ready node, then wait
	// for at least one in-flight node to finish before re-polling
	// readiness. Termination = nothing in flight AND pickReady empty
	// (either every node finished, or remaining nodes are
	// transitively-failed and pickReady skips them).
	for {
		ready := pickReady()
		for _, id := range ready {
			wg.Add(1)
			go runNode(id, byID[id].Kind() != KindGate)
		}
		mu.Lock()
		inflight := len(started) - len(completed)
		mu.Unlock()
		if inflight == 0 {
			// pickReady was empty AND nothing is running → we're done.
			break
		}
		// Brief poll until something completes; runNode's mu.Unlock
		// is the actual wakeup. 1ms is well below DAG step latency
		// and avoids touching wg internals.
		time.Sleep(time.Millisecond)
	}
	wg.Wait()

	result := &PlanResult{
		PlanID:      planID,
		NodeResults: results,
		Errors:      errs,
	}
	if len(errs) > 0 {
		e.publishPlanFailed(planID, plan, result)
		// Return the first error (sorted by node id for determinism).
		var firstErrNodeID string
		for id := range errs {
			if firstErrNodeID == "" || id < firstErrNodeID {
				firstErrNodeID = id
			}
		}
		return result, fmt.Errorf("plan %q failed: node %q: %w", plan.Name, firstErrNodeID, errs[firstErrNodeID])
	}
	e.publishPlanCompleted(planID, plan, result)
	return result, nil
}

// assertAcyclic verifies no cycle exists via Kahn's algorithm.
func assertAcyclic(nodes []Node) error {
	indegree := map[string]int{}
	rev := map[string][]string{}
	for _, n := range nodes {
		indegree[n.ID()] = len(n.DependsOn())
	}
	for _, n := range nodes {
		for _, dep := range n.DependsOn() {
			rev[dep] = append(rev[dep], n.ID())
		}
	}
	var queue []string
	for id, deg := range indegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	processed := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		processed++
		for _, downstream := range rev[id] {
			indegree[downstream]--
			if indegree[downstream] == 0 {
				queue = append(queue, downstream)
			}
		}
	}
	if processed != len(nodes) {
		return ErrCycle
	}
	return nil
}

// ----- event publishers -----

func (e *Executor) publishPlanStarted(planID string, plan Plan) {
	if e.bus == nil {
		return
	}
	_, _ = e.bus.Publish(event.Spec{
		Subject:       "plan." + planID + ".lifecycle",
		Kind:          event.KindPlanStarted,
		Actor:         "scheduler",
		CorrelationID: planID,
		Payload: map[string]any{
			"plan_name":  plan.Name,
			"node_count": len(plan.Nodes),
			"node_ids":   nodeIDs(plan.Nodes),
		},
	})
}

func (e *Executor) publishPlanCompleted(planID string, plan Plan, res *PlanResult) {
	if e.bus == nil {
		return
	}
	_, _ = e.bus.Publish(event.Spec{
		Subject:       "plan." + planID + ".lifecycle",
		Kind:          event.KindPlanCompleted,
		Actor:         "scheduler",
		CorrelationID: planID,
		Payload: map[string]any{
			"plan_name":    plan.Name,
			"node_count":   len(plan.Nodes),
			"results_keys": resultKeys(res.NodeResults),
		},
	})
}

func (e *Executor) publishPlanFailed(planID string, plan Plan, res *PlanResult) {
	if e.bus == nil {
		return
	}
	failed := make([]string, 0, len(res.Errors))
	for id := range res.Errors {
		failed = append(failed, id)
	}
	sort.Strings(failed)
	_, _ = e.bus.Publish(event.Spec{
		Subject:       "plan." + planID + ".lifecycle",
		Kind:          event.KindPlanFailed,
		Actor:         "scheduler",
		CorrelationID: planID,
		Payload: map[string]any{
			"plan_name":  plan.Name,
			"failed_ids": failed,
		},
	})
}

func (e *Executor) publishNodeStarted(planID string, n Node) {
	if e.bus == nil {
		return
	}
	_, _ = e.bus.Publish(event.Spec{
		Subject:       "plan." + planID + ".node." + n.ID(),
		Kind:          event.KindNodeStarted,
		Actor:         "scheduler",
		CorrelationID: planID,
		Payload: map[string]any{
			"node_id":   n.ID(),
			"node_kind": string(n.Kind()),
			"deps":      n.DependsOn(),
		},
	})
}

func (e *Executor) publishNodeCompleted(planID string, n Node, r Result) {
	if e.bus == nil {
		return
	}
	payload := map[string]any{
		"node_id":      n.ID(),
		"node_kind":    string(n.Kind()),
		"output_bytes": len(r.Output),
	}
	maps.Copy(payload, r.Detail)
	_, _ = e.bus.Publish(event.Spec{
		Subject:       "plan." + planID + ".node." + n.ID(),
		Kind:          event.KindNodeCompleted,
		Actor:         "scheduler",
		CorrelationID: planID,
		Payload:       payload,
	})
}

func (e *Executor) publishNodeFailed(planID string, n Node, err error) {
	if e.bus == nil {
		return
	}
	_, _ = e.bus.Publish(event.Spec{
		Subject:       "plan." + planID + ".node." + n.ID(),
		Kind:          event.KindNodeFailed,
		Actor:         "scheduler",
		CorrelationID: planID,
		Payload: map[string]any{
			"node_id":   n.ID(),
			"node_kind": string(n.Kind()),
			"error":     err.Error(),
		},
	})
}

func nodeIDs(nodes []Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.ID()
	}
	sort.Strings(out)
	return out
}

func resultKeys(m map[string]Result) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
