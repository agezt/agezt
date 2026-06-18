// SPDX-License-Identifier: MIT

package scheduler_test

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	intentmodel "github.com/agezt/agezt/kernel/intent"
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

func hasString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
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
	_, err := e.Run(context.Background(), plan, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// max-in-flight directly observes how many nodes ran simultaneously, so
	// >=3 IS the proof that a/b/c fanned out concurrently — deterministic and
	// independent of machine speed. (A wall-clock duration bound was removed: it
	// flaked on slow/loaded CI runners where overhead pushed a genuinely-parallel
	// run past the threshold — e.g. 327ms on a Windows runner — without telling us
	// anything maxInflight doesn't already prove.)
	if maxIn.Load() < 3 {
		t.Errorf("max-in-flight=%d; expected >=3 (parallel a/b/c)", maxIn.Load())
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

func TestRun_InvariantMonitorInvalidatesBeforeNodeStart(t *testing.T) {
	b, j := newBus(t)
	var inflight, maxIn atomic.Int32
	monitorErr := errors.New("world state changed")
	e := scheduler.New(scheduler.Config{
		Bus: b,
		Monitor: func(ctx context.Context, snapshot scheduler.InvariantSnapshot) error {
			if snapshot.Phase == scheduler.InvariantNodeStart && snapshot.NodeID == "a" {
				return monitorErr
			}
			return nil
		},
	})

	plan := scheduler.Plan{
		Name: "guarded",
		Nodes: []scheduler.Node{&fakeNode{
			NodeID: "a", ResultOutput: "must-not-run",
			inflight: &inflight, maxInflight: &maxIn,
		}},
	}
	res, err := e.Run(context.Background(), plan, "")
	if !errors.Is(err, scheduler.ErrPlanInvalidated) {
		t.Fatalf("Run err=%v, want ErrPlanInvalidated", err)
	}
	if !errors.Is(err, monitorErr) {
		t.Fatalf("Run err=%v, want monitor error", err)
	}
	if inflight.Load() != 0 {
		t.Fatal("node Run was entered even though invariant monitor invalidated before node_start")
	}
	if _, ok := res.NodeResults["a"]; ok {
		t.Fatal("invalidated node must not produce a result")
	}
	if !errors.Is(res.Errors["a"], scheduler.ErrPlanInvalidated) {
		t.Fatalf("node error=%v, want ErrPlanInvalidated", res.Errors["a"])
	}
	kinds := countKinds(t, j)
	if kinds[event.KindNodeStarted] != 0 {
		t.Errorf("node.started count=%d want 0", kinds[event.KindNodeStarted])
	}
	if kinds[event.KindNodeFailed] != 1 || kinds[event.KindPlanFailed] != 1 {
		t.Errorf("failure events=%v want one node.failed and one plan.failed", kinds)
	}
}

func TestRun_InvariantMonitorSeesCompletedStateBeforeNextNode(t *testing.T) {
	b, _ := newBus(t)
	var beforeB scheduler.InvariantSnapshot
	e := scheduler.New(scheduler.Config{
		Bus: b,
		Monitor: func(ctx context.Context, snapshot scheduler.InvariantSnapshot) error {
			if snapshot.Phase == scheduler.InvariantNodeStart && snapshot.NodeID == "b" {
				beforeB = snapshot
			}
			return nil
		},
	})

	plan := scheduler.Plan{
		Name: "linear",
		Nodes: []scheduler.Node{
			&fakeNode{NodeID: "a", ResultOutput: "done-a"},
			&fakeNode{NodeID: "b", Deps: []string{"a"}, ResultOutput: "done-b"},
		},
	}
	if _, err := e.Run(context.Background(), plan, "plan-guard"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if beforeB.PlanID != "plan-guard" || beforeB.PlanName != "linear" {
		t.Fatalf("snapshot identity = {PlanID:%q PlanName:%q}", beforeB.PlanID, beforeB.PlanName)
	}
	if !hasString(beforeB.Started, "a") || !hasString(beforeB.Completed, "a") {
		t.Fatalf("snapshot before b = started %v completed %v, want a completed", beforeB.Started, beforeB.Completed)
	}
	if hasString(beforeB.Started, "b") || hasString(beforeB.Completed, "b") {
		t.Fatalf("snapshot before b must be pre-start for b; got started %v completed %v", beforeB.Started, beforeB.Completed)
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

// blockerNode is a compute node that signals on `entered` when it starts and then
// holds its worker slot until `release` is closed — letting a test pin the only
// slot and observe whether other nodes can still make progress.
type blockerNode struct {
	NodeID  string
	Deps    []string
	entered chan struct{}
	release chan struct{}
}

func (n *blockerNode) ID() string             { return n.NodeID }
func (*blockerNode) Kind() scheduler.NodeKind { return scheduler.KindLoop }
func (n *blockerNode) DependsOn() []string    { return n.Deps }
func (n *blockerNode) Run(ctx context.Context, _ scheduler.Inputs) (scheduler.Result, error) {
	select {
	case n.entered <- struct{}{}:
	default:
	}
	select {
	case <-n.release:
	case <-ctx.Done():
	}
	return scheduler.Result{Output: "ran"}, nil
}

// TestGate_DoesNotConsumeComputeSlot pins that a gate awaiting human approval does
// NOT occupy a worker-pool slot. With MaxParallel:1 a compute node holds the only
// slot; the gate must still reach the approval queue while that slot is held. A
// gate that required a slot would be stuck behind the full pool and never become
// pending until the compute node finished — starving the approval for the whole
// run. (The compute node here is what blocks, so this is deterministic regardless
// of goroutine scheduling: only the blocker ever needs the slot.)
func TestGate_DoesNotConsumeComputeSlot(t *testing.T) {
	b, _ := newBus(t)
	apr := approval.New(approval.Config{Bus: b, Timeout: 2 * time.Second})
	e := scheduler.New(scheduler.Config{Bus: b})

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	blocker := &blockerNode{NodeID: "a-blocker", entered: entered, release: release}
	gate := &scheduler.GateNode{NodeID: "b-gate", Approvals: apr, Capability: "plan.execute", Description: "?"}
	plan := scheduler.Plan{Nodes: []scheduler.Node{blocker, gate}, MaxParallel: 1}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{}, 1)
	go func() { _, _ = e.Run(ctx, plan, ""); done <- struct{}{} }()

	// The blocker compute node must occupy the only worker slot.
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		cancel()
		<-done
		t.Fatal("blocker compute node never ran (it should hold the only slot)")
	}

	// While the slot is held, the gate must still become pending — proving it does
	// not require a compute slot.
	pending := false
	for deadline := time.Now().Add(500 * time.Millisecond); time.Now().Before(deadline); {
		if apr.PendingCount() == 1 {
			pending = true
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !pending {
		t.Error("gate did not become pending while a compute node held the only slot: it is waiting for a worker slot it shouldn't need")
	}

	// Teardown: release the blocker and cancel the run (unblocks the gate's wait).
	close(release)
	cancel()
	<-done
}

func TestGateNode_GrantedReleasesDownstream(t *testing.T) {
	b, _ := newBus(t)
	apr := approval.New(approval.Config{Bus: b, Timeout: 2 * time.Second})
	e := scheduler.New(scheduler.Config{Bus: b})
	frame := intentmodel.Frame{
		CanonicalIntent: "clean files",
		HarmfulReading:  "could delete the wrong files",
		AmbiguityScore:  0.8,
		Underdetermined: true,
	}

	gate := &scheduler.GateNode{
		NodeID: "gate", Approvals: apr,
		Capability: "plan.execute", Description: "Allow execute branch?",
		IntentFrame: &frame,
	}
	exec := &fakeNode{NodeID: "execute", Deps: []string{"gate"}, ResultOutput: "did-it"}

	plan := scheduler.Plan{Nodes: []scheduler.Node{gate, exec}}

	// Concurrently grant the pending approval after a short delay.
	go func() {
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if apr.PendingCount() == 1 {
				req := apr.Pending()[0]
				if req.CanonicalIntent != "clean files" || req.HarmfulInterpretation == "" {
					t.Errorf("approval missing intent metadata: %+v", req)
				}
				_ = apr.Resolve(req.ID, approval.DecisionGrant, "ok", "test")
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

// TestGateNode_TimeoutAbortsDownstream locks in the fail-closed property
// of a plan gate: if no operator answers within the approval timeout, the
// gate must synthesise a deny (DecisionTimeout) so the plan aborts rather
// than silently releasing the guarded branch. SPEC-06 §3.4: "Time-outs
// default to deny." Nobody resolves the approval here.
func TestGateNode_TimeoutAbortsDownstream(t *testing.T) {
	b, j := newBus(t)
	apr := approval.New(approval.Config{Bus: b, Timeout: 50 * time.Millisecond})
	e := scheduler.New(scheduler.Config{Bus: b})

	gate := &scheduler.GateNode{
		NodeID: "gate", Approvals: apr, Capability: "plan.execute",
	}
	exec := &fakeNode{NodeID: "execute", Deps: []string{"gate"}, ResultOutput: "must-not-run"}
	plan := scheduler.Plan{Nodes: []scheduler.Node{gate, exec}}

	res, err := e.Run(context.Background(), plan, "")
	if err == nil {
		t.Fatal("expected plan failure after gate timeout")
	}
	if _, ok := res.Errors["gate"]; !ok {
		t.Error("gate should be recorded as a failed node")
	}
	if _, ok := res.NodeResults["execute"]; ok {
		t.Error("execute must not run after gate timeout (fail-closed)")
	}
	kinds := countKinds(t, j)
	if kinds[event.KindApprovalTimeout] != 1 {
		t.Errorf("approval.timeout count=%d want 1", kinds[event.KindApprovalTimeout])
	}
	if kinds[event.KindPlanFailed] != 1 {
		t.Errorf("plan.failed count=%d want 1", kinds[event.KindPlanFailed])
	}
}

// TestGateNode_CancelAbortsDownstream covers the third terminal outcome:
// if the plan's context is cancelled while the gate is waiting for an
// operator, the gate fails (DecisionCancel) and the guarded branch never
// runs. This is the "operator walked away / daemon shutting down" path.
func TestGateNode_CancelAbortsDownstream(t *testing.T) {
	b, _ := newBus(t)
	apr := approval.New(approval.Config{Bus: b, Timeout: 5 * time.Second})
	e := scheduler.New(scheduler.Config{Bus: b})

	gate := &scheduler.GateNode{
		NodeID: "gate", Approvals: apr, Capability: "plan.execute",
	}
	exec := &fakeNode{NodeID: "execute", Deps: []string{"gate"}, ResultOutput: "must-not-run"}
	plan := scheduler.Plan{Nodes: []scheduler.Node{gate, exec}}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel once the gate is parked in the approval queue.
	go func() {
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if apr.PendingCount() == 1 {
				cancel()
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
		t.Errorf("approval never appeared in pending queue")
	}()

	res, err := e.Run(ctx, plan, "")
	if err == nil {
		t.Fatal("expected plan failure after gate cancel")
	}
	if _, ok := res.NodeResults["execute"]; ok {
		t.Error("execute must not run after gate cancel")
	}
}

// TestGateNode_NilApprovalsErrors locks in the misconfiguration guard: a
// gate wired without an approval registry fails the plan rather than
// silently passing (which would defeat the gate entirely).
func TestGateNode_NilApprovalsErrors(t *testing.T) {
	b, _ := newBus(t)
	e := scheduler.New(scheduler.Config{Bus: b})

	gate := &scheduler.GateNode{NodeID: "gate", Capability: "plan.execute"}
	exec := &fakeNode{NodeID: "execute", Deps: []string{"gate"}, ResultOutput: "must-not-run"}
	plan := scheduler.Plan{Nodes: []scheduler.Node{gate, exec}}

	res, err := e.Run(context.Background(), plan, "")
	if err == nil {
		t.Fatal("expected plan failure: gate has no Approvals registry")
	}
	if _, ok := res.NodeResults["execute"]; ok {
		t.Error("execute must not run when the gate is misconfigured")
	}
}

// TestLoopNode_GuardsRejectBadConfig covers the two LoopNode guards: a
// missing Runner and an empty intent both fail the node (and so the plan)
// instead of panicking or running an empty agent loop.
func TestLoopNode_GuardsRejectBadConfig(t *testing.T) {
	t.Run("nil runner", func(t *testing.T) {
		b, _ := newBus(t)
		e := scheduler.New(scheduler.Config{Bus: b})
		plan := scheduler.Plan{Nodes: []scheduler.Node{
			&scheduler.LoopNode{NodeID: "do", Intent: "something"},
		}}
		if _, err := e.Run(context.Background(), plan, ""); err == nil {
			t.Fatal("expected failure: LoopNode has no Runner")
		}
	})
	t.Run("empty intent", func(t *testing.T) {
		b, _ := newBus(t)
		e := scheduler.New(scheduler.Config{Bus: b})
		runner := func(ctx context.Context, intent, corr string) (string, error) {
			return "ran", nil
		}
		plan := scheduler.Plan{Nodes: []scheduler.Node{
			&scheduler.LoopNode{NodeID: "do", Runner: runner},
		}}
		if _, err := e.Run(context.Background(), plan, ""); err == nil {
			t.Fatal("expected failure: LoopNode has empty intent")
		}
	})
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

func TestLoopNode_CarriesIntentFrameToRunner(t *testing.T) {
	b, _ := newBus(t)
	e := scheduler.New(scheduler.Config{Bus: b})
	frame := intentmodel.Frame{
		UserUtteranceHash: "hash",
		CanonicalIntent:   "clean files",
		HarmfulReading:    "could delete the wrong files",
		AmbiguityScore:    0.8,
		Underdetermined:   true,
	}
	var seen intentmodel.Frame
	var ok bool
	runner := func(ctx context.Context, intent, corr string) (string, error) {
		seen, ok = intentmodel.FrameFromContext(ctx)
		return "done", nil
	}
	_, err := e.Run(context.Background(), scheduler.Plan{Nodes: []scheduler.Node{
		&scheduler.LoopNode{NodeID: "do", Intent: "delete files", Runner: runner, IntentFrame: &frame},
	}}, "test-plan-corr")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ok || seen.CanonicalIntent != "clean files" || !seen.Underdetermined {
		t.Fatalf("intent frame not carried to runner: ok=%v frame=%+v", ok, seen)
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

// TestRun_DeepChainEventDrivenDriver exercises the event-driven driver across many
// iterations: a 64-node linear chain completes one node at a time, so the driver
// blocks on the done channel 64 times. A miscount (consuming too many/few signals)
// would deadlock or terminate early; all 64 nodes must complete in order.
func TestRun_DeepChainEventDrivenDriver(t *testing.T) {
	b, _ := newBus(t)
	e := scheduler.New(scheduler.Config{Bus: b})

	const n = 64
	nodes := make([]scheduler.Node, n)
	for i := 0; i < n; i++ {
		id := "n" + strconv.Itoa(i)
		var deps []string
		if i > 0 {
			deps = []string{"n" + strconv.Itoa(i-1)}
		}
		nodes[i] = &fakeNode{NodeID: id, Deps: deps, ResultOutput: id}
	}

	res, err := e.Run(context.Background(), scheduler.Plan{Nodes: nodes}, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.NodeResults) != n {
		t.Fatalf("completed %d of %d nodes — event-driven driver lost or over-consumed a wakeup", len(res.NodeResults), n)
	}
	if got := res.NodeResults["n63"].Output; got != "n63" {
		t.Errorf("last node result=%q want n63", got)
	}
}
