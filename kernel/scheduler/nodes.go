// SPDX-License-Identifier: MIT

package scheduler

import (
	"context"
	"errors"
	"fmt"

	"github.com/ersinkoc/agezt/kernel/agent"
	"github.com/ersinkoc/agezt/kernel/approval"
	"github.com/ersinkoc/agezt/kernel/bus"
)

// LoopRunner is the kernel-side hook that runs one agent tool-loop end
// to end. The runtime supplies a closure that calls agent.Run with the
// kernel's wired config (Provider, Tools, Policy hook, Bus). The
// scheduler doesn't depend on the agent package directly; it just
// hands an intent string in and gets an answer back.
type LoopRunner func(ctx context.Context, intent string, correlationID string) (string, error)

// LoopNode wraps one agent.Run inside a Plan node. A 1-node plan
// containing a single LoopNode is operationally identical to today's
// `agt run "<intent>"`.
type LoopNode struct {
	NodeID   string
	Intent   string
	Deps     []string
	Runner   LoopRunner
	// IntentFn, if set, overrides Intent. The function receives the
	// upstream Inputs so a downstream loop can react to upstream
	// results (e.g. "given the research summary, do the work").
	IntentFn func(Inputs) string
}

// ID implements Node.
func (n *LoopNode) ID() string { return n.NodeID }

// Kind implements Node.
func (*LoopNode) Kind() NodeKind { return KindLoop }

// DependsOn implements Node.
func (n *LoopNode) DependsOn() []string { return n.Deps }

// Run implements Node.
func (n *LoopNode) Run(ctx context.Context, in Inputs) (Result, error) {
	if n.Runner == nil {
		return Result{}, errors.New("loop-node: Runner not set")
	}
	intent := n.Intent
	if n.IntentFn != nil {
		intent = n.IntentFn(in)
	}
	if intent == "" {
		return Result{}, errors.New("loop-node: empty intent")
	}
	// Use the plan's correlationID directly for the loop so the per-
	// loop subject ("agent.<actor>.>") nests under the plan's
	// correlation, keeping `agt why` walkable end-to-end.
	corr := correlationFromCtx(ctx) + ".loop." + n.NodeID
	answer, err := n.Runner(ctx, intent, corr)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Output: answer,
		Detail: map[string]any{"intent": intent, "loop_correlation": corr},
	}, nil
}

// GateNode pauses the plan and routes a request to approval.Registry.
// Grant releases the downstream branch; deny / timeout / cancel cause
// the node to fail and the plan to abort.
type GateNode struct {
	NodeID    string
	Deps      []string
	Approvals *approval.Registry
	// Capability is what the approval is for (e.g. "plan.execute").
	// Surfaces in `agt approvals` so the operator knows what's
	// being asked.
	Capability string
	// Description is a human prompt ("Allow the execute branch to
	// run?"). Surfaces as Request.Reason.
	Description string
	// Actor identifies the originating agent in events; defaults to
	// "scheduler" when empty.
	Actor string
}

// ID implements Node.
func (n *GateNode) ID() string { return n.NodeID }

// Kind implements Node.
func (*GateNode) Kind() NodeKind { return KindGate }

// DependsOn implements Node.
func (n *GateNode) DependsOn() []string { return n.Deps }

// Run implements Node.
func (n *GateNode) Run(ctx context.Context, _ Inputs) (Result, error) {
	if n.Approvals == nil {
		return Result{}, errors.New("gate-node: Approvals registry not set")
	}
	actor := n.Actor
	if actor == "" {
		actor = "scheduler"
	}
	cap := n.Capability
	if cap == "" {
		cap = "plan.gate"
	}
	desc := n.Description
	if desc == "" {
		desc = "scheduler gate-node: " + n.NodeID
	}
	out := n.Approvals.Submit(ctx, approval.SubmitSpec{
		Capability:    cap,
		ToolName:      "scheduler.gate",
		Input:         desc,
		Reason:        desc,
		Actor:         actor,
		CorrelationID: correlationFromCtx(ctx),
	})
	switch out.Decision {
	case approval.DecisionGrant:
		return Result{
			Output: "gate granted by " + out.ResolvedBy,
			Detail: map[string]any{"decision": "grant", "resolved_by": out.ResolvedBy},
		}, nil
	default:
		return Result{
			Detail: map[string]any{"decision": string(out.Decision), "reason": out.Reason},
		}, fmt.Errorf("gate denied: %s (%s)", out.Decision, out.Reason)
	}
}

// ----- ctx propagation helpers -----
//
// The scheduler stashes the plan correlation on its Run context so
// LoopNode/GateNode can inherit it without an explicit field. Re-uses
// the same pattern runtime.policyHook uses for agent correlation.

type ctxKey int

const ctxKeyCorrelation ctxKey = 0

func correlationFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyCorrelation).(string); ok {
		return v
	}
	return ""
}

// withCorrelation returns a child ctx carrying corr; called from the
// executor so node Run methods can pick it up.
func withCorrelation(parent context.Context, corr string) context.Context {
	return context.WithValue(parent, ctxKeyCorrelation, corr)
}

// keep bus import honest (the package uses it for events; this file
// uses agent + approval but compiler doesn't track package-level use).
var _ = bus.Bus{}
var _ = agent.Run
