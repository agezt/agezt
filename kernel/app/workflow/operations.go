// SPDX-License-Identifier: MIT
package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	graphs "github.com/agezt/agezt/kernel/modules/workflows/workflow"
)

type Providers struct {
	Reads     func(context.Context) *Reads
	Lifecycle func(context.Context) *Lifecycle
	Copilot   func(context.Context) *Copilot
	Execution func(context.Context) *Execution
}

// requestGraphRow reads a workflow graph from a request row's own value: the
// row carries the graph as a map, so there is nothing left to read.
func requestGraphRow(value map[string]any) (graphs.Workflow, error) {
	if len(value) == 0 {
		return graphs.Workflow{}, errors.New("args.workflow required")
	}
	return graphValue(value)
}

// graphValue reads a workflow graph from a decoded JSON value.
func graphValue(value any) (graphs.Workflow, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return graphs.Workflow{}, fmt.Errorf("args.workflow: %w", err)
	}
	var graph graphs.Workflow
	if err := json.Unmarshal(encoded, &graph); err != nil {
		return graphs.Workflow{}, fmt.Errorf("args.workflow: %w", err)
	}
	return graph, nil
}

// bindRow binds an operation whose request row is its OWN: the declared input
// is derived from the row's type, so the operation's schema is the row rather
// than a hand-written copy of it.
func bindRow[I, O any](ops *[]app.Operation, name string, read bool, output json.RawMessage, handler func(context.Context, I) (O, error)) error {
	operation, err := app.NewOperation(opapi.Spec{Name: name, ReadOnly: read, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, OutputSchema: output}, handler)
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
			return bindRow(&ops, opapi.WorkflowList, true, nil, func(ctx context.Context, in opapi.WorkflowListRequest) (ListOutput, error) {
				listing := providers.Reads(ctx).PrepareList()
				withRuns, err := opapi.BoolFlagValue(in.WithRuns, "with_runs")
				if err != nil {
					return ListOutput{}, err
				}
				return listing.List(ctx, ListInput{WithRuns: withRuns})
			})
		},
		func() error {
			return bindRow(&ops, opapi.WorkflowShow, true, outputs["show"], func(ctx context.Context, in opapi.WorkflowShowRequest) (ShowOutput, error) {
				ref, err := opapi.RequiredText(in.Ref, "ref")
				if err != nil {
					return ShowOutput{}, err
				}
				return providers.Reads(ctx).Show(ctx, ShowInput{Ref: ref})
			})
		},
		func() error {
			return bindRow(&ops, opapi.WorkflowSave, false, outputs["save"], func(ctx context.Context, in opapi.WorkflowSaveRequest) (SaveOutput, error) {
				graph, err := requestGraphRow(in.Workflow.Map())
				if err != nil {
					return SaveOutput{}, err
				}
				return providers.Lifecycle(ctx).Save(ctx, SaveInput{Workflow: graph})
			})
		},
		func() error {
			return bindRow(&ops, opapi.WorkflowRestore, false, outputs["save"], func(ctx context.Context, in opapi.WorkflowRestoreRequest) (SaveOutput, error) {
				graph, err := requestGraphRow(in.Workflow.Map())
				if err != nil {
					return SaveOutput{}, err
				}
				return providers.Lifecycle(ctx).Restore(ctx, RestoreInput{Workflow: graph, Reason: in.Reason})
			})
		},
		func() error {
			return bindRow(&ops, opapi.WorkflowRemove, false, nil, func(ctx context.Context, in opapi.WorkflowRemoveRequest) (RemoveOutput, error) {
				ref, err := opapi.RequiredText(in.Ref, "ref")
				if err != nil {
					return RemoveOutput{}, err
				}
				return providers.Lifecycle(ctx).Remove(ctx, RemoveInput{Ref: ref})
			})
		},
		func() error {
			return bindRow(&ops, opapi.WorkflowSetEnabled, false, nil, func(ctx context.Context, in opapi.WorkflowSetEnabledRequest) (EnableOutput, error) {
				ref, err := opapi.RequiredText(in.Ref, "ref")
				if err != nil {
					return EnableOutput{}, err
				}
				return providers.Lifecycle(ctx).SetEnabled(ctx, EnableInput{Ref: ref, Enabled: opapi.BoolFlagOrFalse(in.Enabled)})
			})
		},
		func() error {
			return bindRow(&ops, opapi.WorkflowRun, false, nil, func(ctx context.Context, in opapi.WorkflowRunRequest) (RunOutput, error) {
				ref, err := opapi.RequiredText(in.Ref, "ref")
				if err != nil {
					return RunOutput{}, err
				}
				return providers.Execution(ctx).Run(ctx, RunInput{Ref: ref, Payload: in.Payload.Value(), Async: in.Async})
			})
		},
		func() error {
			return bindRow(&ops, opapi.WorkflowDraft, false, outputs["copilot"], func(ctx context.Context, in opapi.WorkflowDraftRequest) (CopilotOutput, error) {
				description, err := opapi.RequiredText(in.Description, "description")
				if err != nil {
					return CopilotOutput{}, err
				}
				return providers.Copilot(ctx).Draft(ctx, DraftInput{Name: in.Name, Description: description})
			})
		},
		func() error {
			return bindRow(&ops, opapi.WorkflowRefine, false, outputs["copilot"], func(ctx context.Context, in opapi.WorkflowRefineRequest) (CopilotOutput, error) {
				instruction, err := opapi.RequiredText(in.Instruction, "instruction")
				if err != nil {
					return CopilotOutput{}, err
				}
				input := RefineInput{Instruction: instruction}
				if len(in.Workflow.Map()) > 0 {
					graph, err := requestGraphRow(in.Workflow.Map())
					if err != nil {
						return CopilotOutput{}, err
					}
					input.Posted = &graph
				} else {
					ref, err := opapi.RequiredText(in.Ref, "ref")
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
			return bindRow(&ops, opapi.WorkflowRuns, true, nil, func(ctx context.Context, in opapi.WorkflowRunsRequest) (RunsOutput, error) {
				ref, err := opapi.RequiredText(in.Ref, "ref")
				if err != nil {
					return RunsOutput{}, err
				}
				history, err := providers.Reads(ctx).PrepareRuns(ref)
				if err != nil {
					return RunsOutput{}, err
				}
				return history.Runs(ctx, RunsInput{Limit: opapi.LimitValue(in.Limit, RunsDefaultLimit)})
			})
		},
		func() error {
			return bindRow(&ops, opapi.WorkflowTemplates, true, outputs["templates"], func(ctx context.Context, _ opapi.WorkflowTemplatesRequest) (TemplatesOutput, error) {
				return providers.Reads(ctx).Templates(ctx, TemplatesInput{})
			})
		},
		func() error {
			return bindRow(&ops, opapi.WorkflowWebhook, false, nil, func(ctx context.Context, in opapi.WorkflowWebhookRequest) (WebhookOutput, error) {
				return providers.Execution(ctx).Webhook(ctx, WebhookInput{Ref: in.Ref, Secret: in.Secret, Payload: in.Payload.Value()})
			})
		},
		func() error {
			return bindRow(&ops, opapi.WorkflowTestNode, false, nil, func(ctx context.Context, in opapi.WorkflowTestNodeRequest) (NodeOutput, error) {
				graph, err := requestGraphRow(in.Workflow.Map())
				if err != nil {
					return NodeOutput{}, err
				}
				node, err := opapi.RequiredText(in.Node, "node")
				if err != nil {
					return NodeOutput{}, err
				}
				return providers.Execution(ctx).TestNode(ctx, NodeInput{Workflow: graph, Node: node, Data: in.Data.Map(), Payload: in.Payload.Value()})
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
