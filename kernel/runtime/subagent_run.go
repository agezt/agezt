// SPDX-License-Identifier: MIT

// Sub-agent entry points on *Kernel: runSubAgent, runSubAgentAsync, awaitSubAgent.
// Code extracted from subagent.go during the Day-40 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)


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
	k.runsMu.Lock()
	if k.halted {
		// Halt won the race after prepare's check: don't start a goroutine
		// Halt's sweep of k.runs can no longer see.
		k.runsMu.Unlock()
		cancel()
		return "", ErrHalted
	}
	k.spawnsMu.Lock()
	k.spawns[p.childCorr] = h
	k.runs[p.childCorr] = cancel
	k.runWG.Add(1) // Close drains async spawns like any in-flight run (M883)
	k.runsMu.Unlock()
	k.spawnsMu.Unlock()
	go func() {
		defer k.runWG.Done()
		defer cancel()
		answer, err := k.executeSubAgent(p)
		h.answer, h.err = answer, err
		k.runsMu.Lock()
		delete(k.runs, p.childCorr)
		k.runsMu.Unlock()
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
	k.spawnsMu.Lock()
	h, ok := k.spawns[spawnID]
	k.spawnsMu.Unlock()
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
	k.spawnsMu.Lock()
	delete(k.spawns, spawnID)
	k.spawnsMu.Unlock()
	if h.err != nil {
		return agent.Result{Output: "delegation failed: " + h.err.Error(), IsError: true}, nil
	}
	return agent.Result{Output: h.answer}, nil
}

// prepareSubAgent resolves and journals one delegation: every bound (depth,
// fan-out, tree total, spend) is enforced HERE, at spawn time, identically
// for sync and async paths, and the subagent.spawned event is published
// before the caller decides how to execute.