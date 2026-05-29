// SPDX-License-Identifier: MIT

package scheduler_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/scheduler"
)

// ---- helpers ----

func newBus(t *testing.T) (*bus.Bus, *journal.Journal) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { j.Close() })
	b := bus.New(j)
	t.Cleanup(b.Close)
	return b, j
}

// fakeNode is a deterministic test node. ResultOutput names what it
// returns; Err makes it fail. Concurrent flag records whether this
// node ran while another node was in flight.
type fakeNode struct {
	NodeID       string
	Deps         []string
	ResultOutput string
	Err          error
	Sleep        time.Duration
	// inflight is a shared atomic that all fakeNodes increment on Run
	// entry and decrement on exit; recorded max gives the actual
	// observed parallelism.
	inflight    *atomic.Int32
	maxInflight *atomic.Int32
}

func (n *fakeNode) ID() string             { return n.NodeID }
func (*fakeNode) Kind() scheduler.NodeKind { return scheduler.KindLoop }
func (n *fakeNode) DependsOn() []string    { return n.Deps }
func (n *fakeNode) Run(ctx context.Context, _ scheduler.Inputs) (scheduler.Result, error) {
	if n.inflight != nil {
		cur := n.inflight.Add(1)
		defer n.inflight.Add(-1)
		for {
			max := n.maxInflight.Load()
			if cur <= max || n.maxInflight.CompareAndSwap(max, cur) {
				break
			}
		}
	}
	if n.Sleep > 0 {
		select {
		case <-time.After(n.Sleep):
		case <-ctx.Done():
			return scheduler.Result{}, ctx.Err()
		}
	}
	if n.Err != nil {
		return scheduler.Result{}, n.Err
	}
	return scheduler.Result{Output: n.ResultOutput}, nil
}

func countKinds(t *testing.T, j *journal.Journal) map[event.Kind]int {
	t.Helper()
	out := map[event.Kind]int{}
	_ = j.Range(func(e *event.Event) error {
		out[e.Kind]++
		return nil
	})
	return out
}

// ---- happy path ----

func TestRun_SingleNodePlan(t *testing.T) {
	b, j := newBus(t)
	e := scheduler.New(scheduler.Config{Bus: b})
	plan := scheduler.Plan{
		Name:  "single",
		Nodes: []scheduler.Node{&fakeNode{NodeID: "a", ResultOutput: "done-a"}},
	}
	res, err := e.Run(context.Background(), plan, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.NodeResults["a"].Output != "done-a" {
		t.Errorf("a output=%q", res.NodeResults["a"].Output)
	}
	if len(res.Errors) != 0 {
		t.Errorf("expected no errors; got %v", res.Errors)
	}
	kinds := countKinds(t, j)
	if kinds[event.KindPlanStarted] != 1 || kinds[event.KindPlanCompleted] != 1 {
		t.Errorf("plan lifecycle events: %v", kinds)
	}
	if kinds[event.KindNodeStarted] != 1 || kinds[event.KindNodeCompleted] != 1 {
		t.Errorf("node lifecycle events: %v", kinds)
	}
}

func TestRun_LinearChainPreservesOrder(t *testing.T) {
	b, _ := newBus(t)
	e := scheduler.New(scheduler.Config{Bus: b})

	var seen []string
	var mu sync.Mutex
	rec := func(id string) *fakeNode {
		return &fakeNode{
			NodeID:       id,
			ResultOutput: id,
			// Each node records its ID under mu so we can verify order
			// after the executor finishes.
		}
	}
	mkRec := func(id string, deps []string) scheduler.Node {
		n := rec(id)
		n.Deps = deps
		return &recordingNode{inner: n, mu: &mu, log: &seen}
	}

	plan := scheduler.Plan{
		Nodes: []scheduler.Node{
			mkRec("a", nil),
			mkRec("b", []string{"a"}),
			mkRec("c", []string{"b"}),
		},
	}
	_, err := e.Run(context.Background(), plan, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(seen) != 3 || seen[0] != "a" || seen[1] != "b" || seen[2] != "c" {
		t.Errorf("execution order=%v want [a b c]", seen)
	}
}

// recordingNode wraps another Node and records its ID at Run time
// under a shared mutex so tests can verify execution order.
type recordingNode struct {
	inner *fakeNode
	mu    *sync.Mutex
	log   *[]string
}

func (n *recordingNode) ID() string               { return n.inner.ID() }
func (n *recordingNode) Kind() scheduler.NodeKind { return n.inner.Kind() }
func (n *recordingNode) DependsOn() []string      { return n.inner.DependsOn() }
func (n *recordingNode) Run(ctx context.Context, in scheduler.Inputs) (scheduler.Result, error) {
	res, err := n.inner.Run(ctx, in)
	n.mu.Lock()
	*n.log = append(*n.log, n.inner.NodeID)
	n.mu.Unlock()
	return res, err
}

// ---- parallelism ----

func TestRun_ParallelBranchesRunConcurrently(t *testing.T) {
	b, _ := newBus(t)
	e := scheduler.New(scheduler.Config{Bus: b})

	var inflight, maxIn atomic.Int32
	mk := func(id string, deps []string) *fakeNode {
		return &fakeNode{
			NodeID: id, Deps: deps, Sleep: 50 * time.Millisecond,
			inflight: &inflight, maxInflight: &maxIn,
		}
	}
	// "root" -> ["a","b","c"] -> "tail"  (3-way fan-out)
	plan := scheduler.Plan{
		Nodes: []scheduler.Node{
			mk("root", nil),
			mk("a", []string{"root"}),
			mk("b", []string{"root"}),
			mk("c", []string{"root"}),
			mk("tail", []string{"a", "b", "c"}),
		},
		MaxParallel: 4,
	}
	start := time.Now()
	_, err := e.Run(context.Background(), plan, "")
	dur := time.Since(start)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if maxIn.Load() < 3 {
		t.Errorf("max-in-flight=%d; expected >=3 (parallel a/b/c)", maxIn.Load())
	}
	// Serialized worst case ≈ 5×50=250ms. Parallel should be ≈ 3×50=150ms.
	if dur > 220*time.Millisecond {
		t.Errorf("dur=%s; expected ≈150ms (fan-out parallel)", dur)
	}
}

// ---- failure handling ----

func TestRun_FailureAbortsDownstreamButNotSiblings(t *testing.T) {
	b, j := newBus(t)
	e := scheduler.New(scheduler.Config{Bus: b})

	plan := scheduler.Plan{
		Nodes: []scheduler.Node{
			&fakeNode{NodeID: "root", ResultOutput: "r"},
			&fakeNode{NodeID: "fail", Deps: []string{"root"}, Err: errors.New("boom")},
			&fakeNode{NodeID: "sibling", Deps: []string{"root"}, ResultOutput: "sib"},
			&fakeNode{NodeID: "downstream", Deps: []string{"fail"}, ResultOutput: "should-not-run"},
		},
	}
	res, err := e.Run(context.Background(), plan, "")
	if err == nil {
		t.Fatal("expected plan to fail")
	}
	if _, ok := res.Errors["fail"]; !ok {
		t.Error("missing error for 'fail'")
	}
	if _, ok := res.NodeResults["sibling"]; !ok {
		t.Error("sibling should have run")
	}
	if _, ok := res.NodeResults["downstream"]; ok {
		t.Error("downstream must NOT run after upstream failure")
	}
	kinds := countKinds(t, j)
	if kinds[event.KindNodeFailed] != 1 {
		t.Errorf("node.failed count=%d want 1", kinds[event.KindNodeFailed])
	}
	if kinds[event.KindPlanFailed] != 1 {
		t.Errorf("plan.failed count=%d want 1", kinds[event.KindPlanFailed])
	}
}

// ---- validation ----

func TestRun_DetectsCycle(t *testing.T) {
	b, _ := newBus(t)
	e := scheduler.New(scheduler.Config{Bus: b})
	plan := scheduler.Plan{
		Nodes: []scheduler.Node{
			&fakeNode{NodeID: "a", Deps: []string{"c"}},
			&fakeNode{NodeID: "b", Deps: []string{"a"}},
			&fakeNode{NodeID: "c", Deps: []string{"b"}},
		},
	}
	_, err := e.Run(context.Background(), plan, "")
	if !errors.Is(err, scheduler.ErrCycle) {
		t.Errorf("got %v, want ErrCycle", err)
	}
}

func TestRun_RejectsDuplicateID(t *testing.T) {
	e := scheduler.New(scheduler.Config{})
	plan := scheduler.Plan{
		Nodes: []scheduler.Node{
			&fakeNode{NodeID: "x"}, &fakeNode{NodeID: "x"},
		},
	}
	_, err := e.Run(context.Background(), plan, "")
	if !errors.Is(err, scheduler.ErrDuplicateNodeID) {
		t.Errorf("got %v, want ErrDuplicateNodeID", err)
	}
}

func TestRun_RejectsUnknownDependency(t *testing.T) {
	e := scheduler.New(scheduler.Config{})
	plan := scheduler.Plan{
		Nodes: []scheduler.Node{
			&fakeNode{NodeID: "a", Deps: []string{"ghost"}},
		},
	}
	_, err := e.Run(context.Background(), plan, "")
	if !errors.Is(err, scheduler.ErrUnknownDependency) {
		t.Errorf("got %v, want ErrUnknownDependency", err)
	}
}

func TestRun_RejectsEmptyPlan(t *testing.T) {
	e := scheduler.New(scheduler.Config{})
	_, err := e.Run(context.Background(), scheduler.Plan{}, "")
	if !errors.Is(err, scheduler.ErrEmptyPlan) {
		t.Errorf("got %v, want ErrEmptyPlan", err)
	}
}

// ---- gate node integration ----

func TestGateNode_GrantedReleasesDownstream(t *testing.T) {
	b, _ := newBus(t)
	apr := approval.New(approval.Config{Bus: b, Timeout: 2 * time.Second})
	e := scheduler.New(scheduler.Config{Bus: b})

	gate := &scheduler.GateNode{
		NodeID: "gate", Approvals: apr,
		Capability: "plan.execute", Description: "Allow execute branch?",
	}
	exec := &fakeNode{NodeID: "execute", Deps: []string{"gate"}, ResultOutput: "did-it"}

	plan := scheduler.Plan{Nodes: []scheduler.Node{gate, exec}}

	// Concurrently grant the pending approval after a short delay.
	go func() {
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if apr.PendingCount() == 1 {
				_ = apr.Resolve(apr.Pending()[0].ID, approval.DecisionGrant, "ok", "test")
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
		t.Errorf("approval never appeared in pending queue")
	}()

	res, err := e.Run(context.Background(), plan, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.NodeResults["execute"].Output != "did-it" {
		t.Errorf("execute didn't run; results=%v", res.NodeResults)
	}
}

func TestGateNode_DeniedAbortsDownstream(t *testing.T) {
	b, _ := newBus(t)
	apr := approval.New(approval.Config{Bus: b, Timeout: 2 * time.Second})
	e := scheduler.New(scheduler.Config{Bus: b})

	gate := &scheduler.GateNode{
		NodeID: "gate", Approvals: apr, Capability: "plan.execute",
	}
	exec := &fakeNode{NodeID: "execute", Deps: []string{"gate"}, ResultOutput: "must-not-run"}
	plan := scheduler.Plan{Nodes: []scheduler.Node{gate, exec}}

	go func() {
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if apr.PendingCount() == 1 {
				_ = apr.Resolve(apr.Pending()[0].ID, approval.DecisionDeny, "nope", "test")
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()

	res, err := e.Run(context.Background(), plan, "")
	if err == nil {
		t.Fatal("expected plan failure after gate deny")
	}
	if _, ok := res.NodeResults["execute"]; ok {
		t.Error("execute must not run after gate deny")
	}
}

// ---- LoopNode integration ----

func TestLoopNode_DelegatesToRunner(t *testing.T) {
	b, _ := newBus(t)
	e := scheduler.New(scheduler.Config{Bus: b})

	var seenIntent string
	var seenCorr string
	runner := func(ctx context.Context, intent, corr string) (string, error) {
		seenIntent = intent
		seenCorr = corr
		return "result-for-" + intent, nil
	}
	plan := scheduler.Plan{
		Nodes: []scheduler.Node{
			&scheduler.LoopNode{
				NodeID: "do", Intent: "hello world", Runner: runner,
			},
		},
	}
	res, err := e.Run(context.Background(), plan, "test-plan-corr")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seenIntent != "hello world" {
		t.Errorf("intent=%q", seenIntent)
	}
	if seenCorr == "" {
		t.Error("loop correlation should be derived from plan correlation")
	}
	if res.NodeResults["do"].Output != "result-for-hello world" {
		t.Errorf("loop output=%q", res.NodeResults["do"].Output)
	}
}

func TestLoopNode_IntentFnReadsUpstream(t *testing.T) {
	b, _ := newBus(t)
	e := scheduler.New(scheduler.Config{Bus: b})

	runner := func(ctx context.Context, intent, corr string) (string, error) {
		return intent + "+ran", nil
	}
	plan := scheduler.Plan{
		Nodes: []scheduler.Node{
			&scheduler.LoopNode{NodeID: "research", Intent: "summarize", Runner: runner},
			&scheduler.LoopNode{
				NodeID: "execute", Deps: []string{"research"}, Runner: runner,
				IntentFn: func(in scheduler.Inputs) string {
					return "given: " + in["research"].Output
				},
			},
		},
	}
	res, err := e.Run(context.Background(), plan, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.NodeResults["execute"].Output != "given: summarize+ran+ran" {
		t.Errorf("execute output=%q", res.NodeResults["execute"].Output)
	}
}
