// SPDX-License-Identifier: MIT
package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	graphs "github.com/agezt/agezt/kernel/workflow"
	"strings"
)

type RequestInput struct {
	Ref         json.RawMessage `json:"ref,omitempty"`
	Workflow    json.RawMessage `json:"workflow,omitempty"`
	Reason      json.RawMessage `json:"reason,omitempty"`
	Enabled     json.RawMessage `json:"enabled,omitempty"`
	Async       json.RawMessage `json:"async,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	WithRuns    json.RawMessage `json:"with_runs,omitempty"`
	Limit       json.RawMessage `json:"limit,omitempty"`
	Name        json.RawMessage `json:"name,omitempty"`
	Description json.RawMessage `json:"description,omitempty"`
	Instruction json.RawMessage `json:"instruction,omitempty"`
	Secret      json.RawMessage `json:"secret,omitempty"`
	Node        json.RawMessage `json:"node,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
}
type Providers struct {
	Reads     func(context.Context) *Reads
	Lifecycle func(context.Context) *Lifecycle
	Copilot   func(context.Context) *Copilot
	Execution func(context.Context) *Execution
}

var requestSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{},"workflow":{},"reason":{},"enabled":{},"async":{},"payload":{},"with_runs":{},"limit":{},"name":{},"description":{},"instruction":{},"secret":{},"node":{},"data":{}}}`)

func requestValue(raw json.RawMessage) any {
	var out any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}
func requestText(raw json.RawMessage, key string) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	value, ok := requestValue(raw).(string)
	if !ok {
		return "", fmt.Errorf("args.%s must be a string", key)
	}
	return value, nil
}
func requiredText(raw json.RawMessage, key string) (string, error) {
	value, err := requestText(raw, key)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("args.%s required", key)
	}
	return value, nil
}
func requestGraph(raw json.RawMessage) (graphs.Workflow, error) {
	if len(raw) == 0 {
		return graphs.Workflow{}, errors.New("args.workflow required")
	}
	encoded, err := json.Marshal(requestValue(raw))
	if err != nil {
		return graphs.Workflow{}, fmt.Errorf("args.workflow: %w", err)
	}
	var graph graphs.Workflow
	if err := json.Unmarshal(encoded, &graph); err != nil {
		return graphs.Workflow{}, fmt.Errorf("args.workflow: %w", err)
	}
	return graph, nil
}
func requestFlag(raw json.RawMessage, key string) (bool, error) {
	if len(raw) == 0 {
		return false, nil
	}
	switch value := requestValue(raw).(type) {
	case bool:
		return value, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "1", "yes":
			return true, nil
		case "false", "0", "no":
			return false, nil
		}
	}
	return false, fmt.Errorf("args.%s must be a boolean", key)
}
func requestEnabled(raw json.RawMessage) bool {
	switch value := requestValue(raw).(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(value, "true") || value == "1"
	}
	return false
}
func requestAsync(raw json.RawMessage) bool { value, _ := requestValue(raw).(bool); return value }
func requestLimit(raw json.RawMessage) int {
	if len(raw) == 0 {
		return RunsDefaultLimit
	}
	value, _ := requestValue(raw).(float64)
	return int(value)
}
func bind[O any](ops *[]app.Operation, name string, read bool, output json.RawMessage, handler func(context.Context, RequestInput) (O, error)) error {
	operation, err := app.NewOperation(opapi.Spec{Name: name, ReadOnly: read, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: requestSchema, OutputSchema: output}, handler)
	if err == nil {
		*ops = append(*ops, operation)
	}
	return err
}
func Operations(providers Providers) ([]app.Operation, error) {
	if providers.Reads == nil || providers.Lifecycle == nil || providers.Copilot == nil || providers.Execution == nil {
		return nil, errors.New("workflow service providers required")
	}
	outputs, err := graphOutputSchemas()
	if err != nil {
		return nil, err
	}
	var ops []app.Operation
	bindings := []func() error{
		func() error {
			return bind(&ops, "workflow_list", true, nil, func(ctx context.Context, in RequestInput) (ListOutput, error) {
				listing := providers.Reads(ctx).PrepareList()
				withRuns, err := requestFlag(in.WithRuns, "with_runs")
				if err != nil {
					return ListOutput{}, err
				}
				return listing.List(ctx, ListInput{WithRuns: withRuns})
			})
		},
		func() error {
			return bind(&ops, "workflow_show", true, outputs["show"], func(ctx context.Context, in RequestInput) (ShowOutput, error) {
				ref, err := requiredText(in.Ref, "ref")
				if err != nil {
					return ShowOutput{}, err
				}
				return providers.Reads(ctx).Show(ctx, ShowInput{Ref: ref})
			})
		},
		func() error {
			return bind(&ops, "workflow_save", false, outputs["save"], func(ctx context.Context, in RequestInput) (SaveOutput, error) {
				graph, err := requestGraph(in.Workflow)
				if err != nil {
					return SaveOutput{}, err
				}
				return providers.Lifecycle(ctx).Save(ctx, SaveInput{Workflow: graph})
			})
		},
		func() error {
			return bind(&ops, "workflow_restore", false, outputs["save"], func(ctx context.Context, in RequestInput) (SaveOutput, error) {
				graph, err := requestGraph(in.Workflow)
				if err != nil {
					return SaveOutput{}, err
				}
				reason, err := requestText(in.Reason, "reason")
				if err != nil {
					return SaveOutput{}, err
				}
				return providers.Lifecycle(ctx).Restore(ctx, RestoreInput{Workflow: graph, Reason: reason})
			})
		},
		func() error {
			return bind(&ops, "workflow_remove", false, nil, func(ctx context.Context, in RequestInput) (RemoveOutput, error) {
				ref, err := requiredText(in.Ref, "ref")
				if err != nil {
					return RemoveOutput{}, err
				}
				return providers.Lifecycle(ctx).Remove(ctx, RemoveInput{Ref: ref})
			})
		},
		func() error {
			return bind(&ops, "workflow_set_enabled", false, nil, func(ctx context.Context, in RequestInput) (EnableOutput, error) {
				ref, err := requiredText(in.Ref, "ref")
				if err != nil {
					return EnableOutput{}, err
				}
				return providers.Lifecycle(ctx).SetEnabled(ctx, EnableInput{Ref: ref, Enabled: requestEnabled(in.Enabled)})
			})
		},
		func() error {
			return bind(&ops, "workflow_run", false, nil, func(ctx context.Context, in RequestInput) (RunOutput, error) {
				ref, err := requiredText(in.Ref, "ref")
				if err != nil {
					return RunOutput{}, err
				}
				return providers.Execution(ctx).Run(ctx, RunInput{Ref: ref, Payload: requestValue(in.Payload), Async: requestAsync(in.Async)})
			})
		},
		func() error {
			return bind(&ops, "workflow_draft", false, outputs["copilot"], func(ctx context.Context, in RequestInput) (CopilotOutput, error) {
				description, err := requiredText(in.Description, "description")
				if err != nil {
					return CopilotOutput{}, err
				}
				name, err := requestText(in.Name, "name")
				if err != nil {
					return CopilotOutput{}, err
				}
				return providers.Copilot(ctx).Draft(ctx, DraftInput{Name: name, Description: description})
			})
		},
		func() error {
			return bind(&ops, "workflow_refine", false, outputs["copilot"], func(ctx context.Context, in RequestInput) (CopilotOutput, error) {
				instruction, err := requiredText(in.Instruction, "instruction")
				if err != nil {
					return CopilotOutput{}, err
				}
				input := RefineInput{Instruction: instruction}
				if len(in.Workflow) > 0 && requestValue(in.Workflow) != nil {
					graph, err := requestGraph(in.Workflow)
					if err != nil {
						return CopilotOutput{}, err
					}
					input.Posted = &graph
				} else {
					ref, err := requestText(in.Ref, "ref")
					if err != nil {
						return CopilotOutput{}, err
					}
					input.Ref = ref
				}
				plan, err := providers.Copilot(ctx).PrepareRefine(input)
				if err != nil {
					return CopilotOutput{}, err
				}
				return plan.Refine(ctx)
			})
		},
		func() error {
			return bind(&ops, "workflow_runs", true, nil, func(ctx context.Context, in RequestInput) (RunsOutput, error) {
				ref, err := requiredText(in.Ref, "ref")
				if err != nil {
					return RunsOutput{}, err
				}
				history, err := providers.Reads(ctx).PrepareRuns(ref)
				if err != nil {
					return RunsOutput{}, err
				}
				return history.Runs(ctx, RunsInput{Limit: requestLimit(in.Limit)})
			})
		},
		func() error {
			return bind(&ops, "workflow_templates", true, outputs["templates"], func(ctx context.Context, _ RequestInput) (TemplatesOutput, error) {
				return providers.Reads(ctx).Templates(ctx, TemplatesInput{})
			})
		},
		func() error {
			return bind(&ops, "workflow_webhook", false, nil, func(ctx context.Context, in RequestInput) (WebhookOutput, error) {
				ref, refErr := requestText(in.Ref, "ref")
				secret, secErr := requestText(in.Secret, "secret")
				return providers.Execution(ctx).Webhook(ctx, WebhookInput{Ref: ref, Secret: secret, Invalid: refErr != nil || secErr != nil, Payload: requestValue(in.Payload)})
			})
		},
		func() error {
			return bind(&ops, "workflow_test_node", false, nil, func(ctx context.Context, in RequestInput) (NodeOutput, error) {
				graph, err := requestGraph(in.Workflow)
				if err != nil {
					return NodeOutput{}, err
				}
				node, err := requiredText(in.Node, "node")
				if err != nil {
					return NodeOutput{}, err
				}
				var data map[string]any
				if len(in.Data) > 0 {
					var ok bool
					data, ok = requestValue(in.Data).(map[string]any)
					if !ok {
						return NodeOutput{}, errors.New("args.data must be an object")
					}
				}
				return providers.Execution(ctx).TestNode(ctx, NodeInput{Workflow: graph, Node: node, Data: data, Payload: requestValue(in.Payload)})
			})
		},
	}
	for _, binding := range bindings {
		if err := binding(); err != nil {
			return nil, err
		}
	}
	return ops, nil
}
