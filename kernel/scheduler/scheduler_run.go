// SPDX-License-Identifier: MIT

// Scheduler run: Executor.Run (the big 250-line runner) + assertAcyclic.
// Code extracted from scheduler.go during the Day-64 god-file split. Public API unchanged.
package scheduler


import (
	"context"
	"fmt"
	"github.com/agezt/agezt/kernel/ulid"
	"sort"
	"sync"
)


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
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	runCtx = withCorrelation(runCtx, planID)
	e.publishPlanStarted(planID, plan)

	// State for the dynamic scheduler.
	var (
		mu          sync.Mutex
		results     = map[string]Result{}
		errs        = map[string]error{}
		completed   = map[string]struct{}{}
		started     = map[string]struct{}{}
		invalidated bool
	)
	checkInvariant := func(phase InvariantPhase, nodeID string) error {
		if e.monitor == nil {
			return nil
		}
		mu.Lock()
		snapshot := invariantSnapshot(planID, plan, phase, nodeID, started, completed, errs)
		mu.Unlock()
		if err := e.monitor(runCtx, snapshot); err != nil {
			return fmt.Errorf("%w: %w", ErrPlanInvalidated, err)
		}
		return nil
	}

	// indegree counts how many unmet deps each node has.
	indegree := make(map[string]int, len(plan.Nodes))
	for _, n := range plan.Nodes {
		indegree[n.ID()] = len(n.DependsOn())
	}

	sem := make(chan struct{}, maxParallel)
	// done signals one node completion to the driver, replacing a 1 ms busy-wait
	// poll. Buffered to the node count so a completing node never blocks sending
	// (and a late send after the driver has moved on is simply discarded).
	done := make(chan struct{}, len(plan.Nodes))
	var wg sync.WaitGroup
	failNodeWithoutRun := func(id string, err error) {
		mu.Lock()
		invalidated = true
		started[id] = struct{}{}
		completed[id] = struct{}{}
		errs[id] = err
		for _, m := range plan.Nodes {
			for _, dep := range m.DependsOn() {
				if dep == id {
					indegree[m.ID()]--
				}
			}
		}
		mu.Unlock()
		e.publishNodeFailed(planID, byID[id], err)
		done <- struct{}{}
	}

	if err := checkInvariant(InvariantPlanStart, ""); err != nil {
		result := &PlanResult{
			PlanID:      planID,
			NodeResults: results,
			Errors:      map[string]error{"plan": err},
		}
		e.publishPlanFailed(planID, plan, result)
		return result, err
	}

	// readyToRun returns IDs that have indegree 0 and have not started
	// yet, and that have no failed dependency.
	pickReady := func() []string {
		mu.Lock()
		defer mu.Unlock()
		if invalidated {
			return nil
		}
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
		// Wake the driver to re-poll readiness (completed[id] is already recorded
		// above). Buffered, so this never blocks.
		done <- struct{}{}
	}

	// Drive the scheduler: launch every newly-ready node, then wait
	// for at least one in-flight node to finish before re-polling
	// readiness. Termination = nothing in flight AND pickReady empty
	// (either every node finished, or remaining nodes are
	// transitively-failed and pickReady skips them).
	for {
		ready := pickReady()
		for _, id := range ready {
			if err := checkInvariant(InvariantNodeStart, id); err != nil {
				failNodeWithoutRun(id, err)
				cancel()
				break
			}
			mu.Lock()
			started[id] = struct{}{}
			mu.Unlock()
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
		// Block until at least one in-flight node completes, then re-poll
		// readiness. Event-driven (no busy-wait, no scheduling-latency floor); the
		// buffered channel and the inflight>0 guard guarantee a send is pending or
		// coming, so this never deadlocks.
		<-done
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