// SPDX-License-Identifier: MIT

package delegation

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
)

// Service is the delegation engine: it resolves a delegation into a Plan,
// enforces the bounds, journals the spawn, runs the child (with the retry
// policy) and keeps the async spawn handles.
//
// The service owns three maps that used to live in the daemon: the fan-out
// tally (M46), the tree-total tally (M629) and the async spawn handles
// (M881). They are delegation state — no root run ever touches them — and
// the daemon releases them through ReleaseTallies and CancelSpawns.
type Service struct {
	host Host
	deps Deps

	// mu guards fanout, tree and spawns. It is a LEAF lock: the module
	// never calls back into the daemon while holding it.
	mu     sync.Mutex
	fanout map[string]int          // spawning correlation_id → sub-agents spawned (M46)
	tree   map[string]int          // root correlation_id → total sub-agents in the tree (M629)
	spawns map[string]*spawnHandle // child correlation_id → pending/finished async delegation (M881)
}

// New builds the delegation engine. The deps must carry a Host, a Config
// provider and a RunChild callback; the module refuses to work without them
// rather than failing later on a nil.
func New(deps Deps) *Service {
	return &Service{
		host:   deps.Host,
		deps:   deps,
		fanout: map[string]int{},
		tree:   map[string]int{},
		spawns: map[string]*spawnHandle{},
	}
}

// Run executes a delegation synchronously: the caller blocks until the child
// returns its answer.
func (s *Service) Run(ctx context.Context, req Request) (string, error) {
	plan, err := s.prepare(ctx, req, false)
	if err != nil {
		return "", err
	}
	return s.execute(plan)
}

// Spawn starts a delegation asynchronously and returns the child's
// correlation id (its spawn id) immediately. The child keeps running after
// the spawning tool call returns; the result is collected with Await.
func (s *Service) Spawn(ctx context.Context, req Request) (string, error) {
	plan, err := s.prepare(ctx, req, true)
	if err != nil {
		return "", err
	}
	return s.spawn(plan)
}

// Await blocks until an async delegation finishes (or the calling tool
// context ends) and returns its result exactly once. Only the spawning run
// may collect — a foreign correlation asking for someone else's spawn id is
// refused.
func (s *Service) Await(ctx context.Context, spawnID string) (toolapi.Result, error) {
	spawnID = strings.TrimSpace(spawnID)
	if spawnID == "" {
		return toolapi.Result{Output: "spawn_id required", IsError: true}, nil
	}
	s.mu.Lock()
	h, ok := s.spawns[spawnID]
	s.mu.Unlock()
	if !ok {
		return toolapi.Result{Output: fmt.Sprintf("unknown spawn id %q (already collected, cancelled, or never spawned)", spawnID), IsError: true}, nil
	}
	if caller := runctx.CorrelationFromCtx(ctx); caller != "" && caller != h.parentCorr {
		return toolapi.Result{Output: "spawn " + spawnID + " belongs to another run", IsError: true}, nil
	}
	select {
	case <-ctx.Done():
		// The per-tool timeout (or a run-level cancel) fired while the child
		// is still working. The handle stays collectable: the model can call
		// delegate_await again; a genuine run cancel ends the loop upstream.
		return toolapi.Result{Output: "sub-agent " + spawnID + " is still running — call delegate_await again to keep waiting", IsError: true}, nil
	case <-h.done:
	}
	s.mu.Lock()
	delete(s.spawns, spawnID)
	s.mu.Unlock()
	if h.err != nil {
		return toolapi.Result{Output: "delegation failed: " + h.err.Error(), IsError: true}, nil
	}
	return toolapi.Result{Output: h.answer}, nil
}

// ReleaseTallies drops the fan-out and tree-total tallies of a finished
// run, so a long-lived daemon does not accumulate them. The daemon calls it
// when a run ends (the daemon's own cleanup path).
func (s *Service) ReleaseTallies(corr string) {
	s.mu.Lock()
	delete(s.fanout, corr)
	delete(s.tree, corr)
	s.mu.Unlock()
}

// CancelSpawns cancels the still-pending async delegations of a run or of its
// delegation tree: an un-awaited child must not outlive the run that spawned
// it. It returns the orphan cancel funcs; the caller invokes them.
func (s *Service) CancelSpawns(corr string) []context.CancelFunc {
	s.mu.Lock()
	var orphans []context.CancelFunc
	for id, h := range s.spawns {
		if h.rootCorr == corr || h.parentCorr == corr {
			orphans = append(orphans, h.cancel)
			delete(s.spawns, id)
		}
	}
	s.mu.Unlock()
	return orphans
}
