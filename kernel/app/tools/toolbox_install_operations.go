// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/toolbox"
	"reflect"
)

type ToolboxInstallRequest struct {
	Names json.RawMessage `json:"names,omitempty"`
}

var toolboxInstallSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"names":{}}}`)

// toolboxProgressSchema specializes the shared event envelope with this producer's
// InstallResult payload and its required tool/ok fields.
var toolboxProgressSchema = func() json.RawMessage {
	var frame map[string]any
	if err := json.Unmarshal([]byte(event.WireSchema), &frame); err != nil {
		panic(err)
	}
	raw, err := schema.FromType(reflect.TypeFor[toolbox.InstallResult](), true)
	if err != nil {
		panic(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		panic(err)
	}
	frame["properties"].(map[string]any)["payload"] = payload
	frame["required"] = append(frame["required"].([]any), "payload")
	out, err := json.Marshal(frame)
	if err != nil {
		panic(err)
	}
	return out
}()

func toolboxInstallNames(raw json.RawMessage) []string {
	var value any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &value)
	}
	list, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if name, ok := item.(string); ok && name != "" {
			out = append(out, name)
		}
	}
	return out
}
func ToolboxInstallOperations(provider func(context.Context) *ToolboxInstall) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("toolbox installer provider required")
	}
	operation, err := app.NewStreamingOperation(opapi.Spec{Name: "toolbox_install", Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, Stream: opapi.StreamEvents, AllowUnknownInput: true, InputSchema: toolboxInstallSchema, EmissionSchema: toolboxProgressSchema}, func(ctx context.Context, in ToolboxInstallRequest, emit func(event.Event) error) (ToolboxInstallOutput, error) {
		names := toolboxInstallNames(in.Names)
		if len(names) == 0 {
			return ToolboxInstallOutput{}, errors.New("args.names (non-empty list) required")
		}
		return provider(ctx).Install(ctx, ToolboxInstallInput{Names: names}, emit)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{operation}, nil
}
