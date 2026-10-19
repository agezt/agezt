// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	intentmodel "github.com/agezt/agezt/kernel/intent"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/scheduler"
)

// PlanSubscription delivers the plan's events until it is cancelled.
type PlanSubscription interface {
	Events() <-chan *event.Event
	Cancel()
}

// BusSubscription adapts a bus subscription to the PlanSubscription handle.
type BusSubscription struct{ *bus.Subscription }

// Events delivers the matched events in publish order.
func (b BusSubscription) Events() <-chan *event.Event { return b.C }

// Cancel releases the subscription.
func (b BusSubscription) Cancel() { b.Subscription.Cancel() }

// PlanRunner executes a submitted plan and streams its events to the caller.
//
// The plan is a wire shape: the operator sends the JSON a planner returned,
// and the runner reifies it into a scheduler.Plan with the kernel's wired
// LoopRunner and Approvals registry.
type PlanRunner struct {
	loopRunner func() scheduler.LoopRunner
	approvals  func() *approval.Registry
	runPlan    func(ctx context.Context, plan scheduler.Plan, planID string) (*scheduler.PlanResult, error)
	subscribe  func(pattern string, buffer int) (PlanSubscription, error)
}

// NewPlanRunner binds the plan runner to the kernel.
func NewPlanRunner(loopRunner func() scheduler.LoopRunner, approvals func() *approval.Registry, runPlan func(ctx context.Context, plan scheduler.Plan, planID string) (*scheduler.PlanResult, error), subscribe func(pattern string, buffer int) (PlanSubscription, error)) *PlanRunner {
	return &PlanRunner{loopRunner: loopRunner, approvals: approvals, runPlan: runPlan, subscribe: subscribe}
}

// PlanRunRequest is the plan submission.
type PlanRunRequest struct {
	PlanJSON json.RawMessage `json:"plan_json,omitempty"`
}

// PlanRunOutput is the plan's result: its id and each node's output.
type PlanRunOutput struct {
	PlanID      string         `json:"plan_id"`
	NodeOutputs map[string]any `json:"node_outputs"`
}

// planSpec is the JSON shape a client submits.
type planSpec struct {
	Name        string             `json:"name"`
	MaxParallel int                `json:"max_parallel"`
	Intent      *intentmodel.Frame `json:"intent,omitempty"`
	Nodes       []planNodeSpec     `json:"nodes"`
}

// planNodeSpec is one submitted node.
type planNodeSpec struct {
	ID   string   `json:"id"`
	Kind string   `json:"kind"`
	Deps []string `json:"deps,omitempty"`
	// Loop fields.
	Intent string `json:"intent,omitempty"`
	// Gate fields.
	Capability  string `json:"capability,omitempty"`
	Description string `json:"description,omitempty"`
}

// Run executes the submitted plan and streams its events. The events are the
// plan's own: the caller sees plan.started and every node.* event in order,
// and the terminal answer carries the node outputs.
func (p *PlanRunner) Run(ctx context.Context, in PlanRunRequest, emit func(*event.Event) error) (PlanRunOutput, error) {
	raw, err := planJSONArg(in.PlanJSON)
	if err != nil {
		return PlanRunOutput{}, err
	}
	var spec planSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return PlanRunOutput{}, fmt.Errorf("plan_json parse: %w", err)
	}
	if len(spec.Nodes) == 0 {
		return PlanRunOutput{}, fmt.Errorf("plan has no nodes")
	}
	plan, err := p.build(spec)
	if err != nil {
		return PlanRunOutput{}, err
	}
	planID := planIDFor(app.RequestID(ctx))

	// Subscribe to the plan's events before launching so the caller sees
	// plan.started and every node.* event in order.
	sub, err := p.subscribe("plan.>", 1024)
	if err != nil {
		return PlanRunOutput{}, err
	}
	defer sub.Cancel()

	type planResult struct {
		res *scheduler.PlanResult
		err error
	}
	resultCh := make(chan planResult, 1)
	go func() {
		r, err := p.runPlan(ctx, plan, planID)
		resultCh <- planResult{r, err}
	}()

	for {
		select {
		case ev, ok := <-sub.Events():
			if !ok {
				return PlanRunOutput{}, fmt.Errorf("event subscription closed")
			}
			if err := emit(ev); err != nil {
				return PlanRunOutput{}, err
			}
		case r := <-resultCh:
			// Drain the in-flight events before the terminal answer.
			for {
				select {
				case ev := <-sub.Events():
					if ev == nil {
						goto drained
					}
					if err := emit(ev); err != nil {
						return PlanRunOutput{}, err
					}
				default:
					goto drained
				}
			}
		drained:
			if r.err != nil {
				return PlanRunOutput{}, r.err
			}
			outputs := map[string]any{}
			for id, nr := range r.res.NodeResults {
				outputs[id] = nr.Output
			}
			return PlanRunOutput{PlanID: r.res.PlanID, NodeOutputs: outputs}, nil
		case <-ctx.Done():
			return PlanRunOutput{}, ctx.Err()
		}
	}
}

// build reifies the submitted plan into a scheduler plan.
func (p *PlanRunner) build(spec planSpec) (scheduler.Plan, error) {
	runner := p.loopRunner()
	apr := p.approvals()
	nodes := make([]scheduler.Node, 0, len(spec.Nodes))
	for _, ns := range spec.Nodes {
		switch ns.Kind {
		case "loop":
			nodes = append(nodes, &scheduler.LoopNode{
				NodeID: ns.ID, Deps: ns.Deps, Intent: ns.Intent, Runner: runner, IntentFrame: spec.Intent,
			})
		case "gate":
			nodes = append(nodes, &scheduler.GateNode{
				NodeID: ns.ID, Deps: ns.Deps, Approvals: apr,
				Capability: ns.Capability, Description: ns.Description, IntentFrame: spec.Intent,
			})
		default:
			return scheduler.Plan{}, fmt.Errorf("node %q: unknown kind %q (want loop|gate)", ns.ID, ns.Kind)
		}
	}
	return scheduler.Plan{Name: spec.Name, MaxParallel: spec.MaxParallel, Nodes: nodes}, nil
}

// planJSONArg reads the submitted plan JSON: a string, as the CLI and the Web
// UI both send it.
func planJSONArg(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("args.plan_json required (JSON string)")
	}
	var s *string
	if err := json.Unmarshal(raw, &s); err != nil || s == nil {
		return "", fmt.Errorf("args.plan_json must be a JSON string")
	}
	return *s, nil
}

// planIDFor names a plan after the request that submitted it.
func planIDFor(requestID string) string {
	planID := "plan-" + strings.TrimPrefix(requestID, "q")
	if planID == "plan-" {
		return ""
	}
	return planID
}

// PlanRunOperations declares the plan execution: audited, operator-only and
// live, so a disconnected client cancels the plan instead of running it into a
// closed connection. It streams the plan's own event frames.
func PlanRunOperations(provider func(context.Context) *PlanRunner) ([]app.Operation, error) {
	if provider == nil {
		return nil, fmt.Errorf("plan runner provider required")
	}
	out, err := schema.FromType(reflect.TypeFor[PlanRunOutput](), false)
	if err != nil {
		return nil, err
	}
	frames := json.RawMessage(event.WireSchema)
	run, err := app.NewStreamingOperation(opapi.Spec{Name: "plan", Stream: opapi.StreamEvents, OutputSchema: out, EmissionSchema: frames, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"plan_json":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/plan/run"}}, func(ctx context.Context, in PlanRunRequest, emit func(*event.Event) error) (PlanRunOutput, error) {
		return provider(ctx).Run(ctx, in, emit)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{run}, nil
}
