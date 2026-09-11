// SPDX-License-Identifier: MIT

// Scheduler core: types (Plan/Node/Result/Invariant*) + Config + New + ContextInvariantMonitor.
// Code extracted from scheduler.go during the Day-64 god-file split. Public API unchanged.
package scheduler


import (
	"context"
	"errors"
	"time"

	"github.com/agezt/agezt/kernel/bus"
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

// InvariantPhase names the scheduler boundary at which a plan invariant is
// checked. The monitor is intentionally separate from the planner: it observes
// committed execution state and may invalidate the plan before more work starts.
type InvariantPhase string

const (
	InvariantPlanStart InvariantPhase = "plan_start"
	InvariantNodeStart InvariantPhase = "node_start"
)

// InvariantSnapshot is the bounded execution state exposed to an
// InvariantMonitor. It deliberately excludes node outputs; monitors should guard
// resource/world-state constraints, not inspect arbitrary result payloads.
type InvariantSnapshot struct {
	PlanID    string
	PlanName  string
	Phase     InvariantPhase
	NodeID    string
	Started   []string
	Completed []string
	Failed    []string
}

// InvariantMonitor is called at plan start and immediately before each node is
// allowed to start. Returning an error invalidates the plan.
type InvariantMonitor func(ctx context.Context, snapshot InvariantSnapshot) error

// Executor runs a Plan. One Executor instance per kernel; safe for
// concurrent Run calls (each plan gets its own correlation_id).
type Executor struct {
	bus     *bus.Bus
	now     func() time.Time
	monitor InvariantMonitor
}

// Config configures an Executor.
type Config struct {
	Bus     *bus.Bus
	Now     func() time.Time
	Monitor InvariantMonitor
}

// New constructs an Executor.
func New(cfg Config) *Executor {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Executor{bus: cfg.Bus, now: now, monitor: cfg.Monitor}
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

// ErrPlanInvalidated is returned when an InvariantMonitor rejects the committed
// plan state before the next boundary is allowed to run.
var ErrPlanInvalidated = errors.New("scheduler: plan invalidated")

// ContextInvariantMonitor invalidates a plan when its context has already been
// cancelled. Runtime wires this as the baseline monitor so Halt/CancelRun cannot
// allow newly-ready nodes to start after cancellation has propagated.
func ContextInvariantMonitor(ctx context.Context, _ InvariantSnapshot) error {
	return ctx.Err()
}

// Run executes the plan to completion or the first node failure.
// CorrelationID, if empty, is generated.