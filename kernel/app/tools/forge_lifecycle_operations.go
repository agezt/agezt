// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/toolforge"
)

type ForgeDraftRequest struct {
	Tool json.RawMessage `json:"tool,omitempty"`
}
type ForgeEditRequest struct {
	Ref  json.RawMessage `json:"ref,omitempty"`
	Tool json.RawMessage `json:"tool,omitempty"`
}
type ForgeTestRequest struct {
	Ref   json.RawMessage `json:"ref,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}
type ForgeRefRequest struct {
	Ref json.RawMessage `json:"ref,omitempty"`
}
type ForgeQuarantineRequest struct {
	Ref    json.RawMessage `json:"ref,omitempty"`
	Reason json.RawMessage `json:"reason,omitempty"`
}

var forgeMutationSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{},"tool":{},"input":{},"reason":{}}}`)

func forgeDecodeTool(raw json.RawMessage) (toolforge.ScriptTool, error) {
	if len(raw) == 0 {
		return toolforge.ScriptTool{}, errors.New("args.tool required")
	}
	var tool toolforge.ScriptTool
	if err := json.Unmarshal(raw, &tool); err != nil {
		return toolforge.ScriptTool{}, fmt.Errorf("args.tool: %w", err)
	}
	return tool, nil
}
func forgeOptionalText(raw json.RawMessage, key string) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("args.%s must be a string", key)
	}
	return text, nil
}
func ForgeLifecycleOperations(provider func(context.Context) *ForgeLifecycle) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("forge lifecycle provider required")
	}
	spec := func(name string) opapi.Spec {
		return opapi.Spec{Name: name, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: forgeMutationSchema}
	}
	draft, err := app.NewOperation(spec("toolforge_draft"), func(ctx context.Context, in ForgeDraftRequest) (ForgeMutationOutput, error) {
		tool, err := forgeDecodeTool(in.Tool)
		if err != nil {
			return ForgeMutationOutput{}, err
		}
		return provider(ctx).Draft(ctx, ForgeDraftInput{Tool: tool})
	})
	if err != nil {
		return nil, err
	}
	edit, err := app.NewOperation(spec("toolforge_edit"), func(ctx context.Context, in ForgeEditRequest) (ForgeMutationOutput, error) {
		ref, err := forgeRequiredRef(in.Ref)
		if err != nil {
			return ForgeMutationOutput{}, err
		}
		tool, err := forgeDecodeTool(in.Tool)
		if err != nil {
			return ForgeMutationOutput{}, err
		}
		return provider(ctx).Edit(ctx, ForgeEditInput{Ref: ref, Tool: tool})
	})
	if err != nil {
		return nil, err
	}
	test, err := app.NewOperation(spec("toolforge_test"), func(ctx context.Context, in ForgeTestRequest) (ForgeTestOutput, error) {
		ref, err := forgeRequiredRef(in.Ref)
		if err != nil {
			return ForgeTestOutput{}, err
		}
		sample, err := forgeOptionalText(in.Input, "input")
		if err != nil {
			return ForgeTestOutput{}, err
		}
		return provider(ctx).Test(ctx, ForgeTestInput{Ref: ref, Sample: sample})
	})
	if err != nil {
		return nil, err
	}
	promote, err := app.NewOperation(spec("toolforge_promote"), func(ctx context.Context, in ForgeRefRequest) (ForgeMutationOutput, error) {
		ref, err := forgeRequiredRef(in.Ref)
		if err != nil {
			return ForgeMutationOutput{}, err
		}
		return provider(ctx).Promote(ctx, ForgeRefInput{Ref: ref})
	})
	if err != nil {
		return nil, err
	}
	quarantine, err := app.NewOperation(spec("toolforge_quarantine"), func(ctx context.Context, in ForgeQuarantineRequest) (ForgeMutationOutput, error) {
		ref, err := forgeRequiredRef(in.Ref)
		if err != nil {
			return ForgeMutationOutput{}, err
		}
		reason, err := forgeOptionalText(in.Reason, "reason")
		if err != nil {
			return ForgeMutationOutput{}, err
		}
		return provider(ctx).Quarantine(ctx, ForgeQuarantineInput{Ref: ref, Reason: reason})
	})
	if err != nil {
		return nil, err
	}
	remove, err := app.NewOperation(spec("toolforge_remove"), func(ctx context.Context, in ForgeRefRequest) (ForgeRemoveOutput, error) {
		ref, err := forgeRequiredRef(in.Ref)
		if err != nil {
			return ForgeRemoveOutput{}, err
		}
		return provider(ctx).Remove(ctx, ForgeRefInput{Ref: ref})
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{draft, edit, test, promote, quarantine, remove}, nil
}
