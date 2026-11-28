// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/planner"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// PlanAuthoring generates a plan from an intent and revises an existing one.
// It is the operator's two-step plan surface: review a generated plan, then
// ask for a revision with a sentence of feedback. There is no automated
// LLM-to-LLM cascade — a human is in the loop on every revision.
type PlanAuthoring struct {
	provider func() llm.Provider
	generate func(ctx context.Context, cfg planner.Config, intent string) (string, planner.Plan, error)
	refine   func(ctx context.Context, cfg planner.Config, original planner.Plan, feedback string) (string, planner.Plan, error)
}

// PlanOutput is a generated or revised plan: its JSON and its node count.
type PlanOutput struct {
	PlanJSON  string `json:"plan_json"`
	NodeCount int    `json:"node_count"`
}

// NewPlanAuthoring binds the plan authoring surface to the kernel's provider
// and the planner.
func NewPlanAuthoring(provider func() llm.Provider) *PlanAuthoring {
	return &PlanAuthoring{
		provider: provider,
		generate: planner.Generate,
		refine:   planner.Refine,
	}
}

// config builds the planner configuration for a request. The model override is
// the caller's own string; an empty one falls back to the provider's default.
func (p *PlanAuthoring) config(model string) (planner.Config, error) {
	var provider llm.Provider
	if p.provider != nil {
		provider = p.provider()
	}
	if provider == nil {
		return planner.Config{}, fmt.Errorf("daemon has no provider configured")
	}
	return planner.Config{Provider: provider, Model: model}, nil
}

// Generate answers with a validated plan JSON and its node count.
func (p *PlanAuthoring) Generate(ctx context.Context, in opapi.PlanGenerateRequest) (PlanOutput, error) {
	intent, err := requiredText(in.Intent, "intent")
	if err != nil {
		return PlanOutput{}, err
	}
	cfg, err := p.config(in.Model)
	if err != nil {
		return PlanOutput{}, err
	}
	rawJSON, plan, err := p.generate(ctx, cfg, intent)
	if err != nil {
		return PlanOutput{}, err
	}
	return PlanOutput{PlanJSON: rawJSON, NodeCount: len(plan.Nodes)}, nil
}

// Refine answers with the revised plan JSON and its node count.
func (p *PlanAuthoring) Refine(ctx context.Context, in opapi.PlanRefineRequest) (PlanOutput, error) {
	planJSON, err := requiredText(in.PlanJSON, "plan_json")
	if err != nil {
		return PlanOutput{}, err
	}
	feedback, err := requiredText(in.Feedback, "feedback")
	if err != nil {
		return PlanOutput{}, err
	}
	var original planner.Plan
	if err := json.Unmarshal([]byte(planJSON), &original); err != nil {
		return PlanOutput{}, fmt.Errorf("plan_json: %w", err)
	}
	cfg, err := p.config(in.Model)
	if err != nil {
		return PlanOutput{}, err
	}
	rawJSON, revised, err := p.refine(ctx, cfg, original, feedback)
	if err != nil {
		return PlanOutput{}, err
	}
	return PlanOutput{PlanJSON: rawJSON, NodeCount: len(revised.Nodes)}, nil
}

// requiredText is a strict required text: a blank one is refused, and the one
// that was sent is returned as it came.
func requiredText(text, key string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("args.%s required", key)
	}
	return text, nil
}

// PlanAuthoringOperations declares the plan authoring surface: audited,
// operator-only and live, so a disconnected client cancels the model call
// instead of spending it into a closed connection. They send no event frames:
// the Web UI follows a run on the bus under the correlation it chose.
func PlanAuthoringOperations(provider func(context.Context) *PlanAuthoring) ([]app.Operation, error) {
	if provider == nil {
		return nil, fmt.Errorf("plan authoring provider required")
	}
	out, err := schema.FromType(reflect.TypeFor[PlanOutput](), false)
	if err != nil {
		return nil, err
	}
	frames := json.RawMessage(event.WireSchema)
	generate, err := app.NewStreamingOperation(opapi.Spec{Name: opapi.PlanGenerate, Stream: opapi.StreamLive, OutputSchema: out, EmissionSchema: frames, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, in opapi.PlanGenerateRequest, _ func(event.Event) error) (PlanOutput, error) {
		return provider(ctx).Generate(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	refine, err := app.NewStreamingOperation(opapi.Spec{Name: opapi.PlanRefine, Stream: opapi.StreamLive, OutputSchema: out, EmissionSchema: frames, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, in opapi.PlanRefineRequest, _ func(event.Event) error) (PlanOutput, error) {
		return provider(ctx).Refine(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{generate, refine}, nil
}
