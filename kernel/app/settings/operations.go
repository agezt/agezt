// SPDX-License-Identifier: MIT
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"strings"
)

type RegisterRequest struct {
	Section json.RawMessage `json:"section,omitempty"`
}
type UnregisterRequest struct {
	ID    json.RawMessage `json:"id,omitempty"`
	Force json.RawMessage `json:"force,omitempty"`
}

// settingsInputSchema declares what the door accepts before a settings
// command runs. The types are the ones the codecs read: name, value and id
// are strings (settingsText), force is a boolean (settingsBool) and section
// is the object the registry decodes (json.Unmarshal into settings.Section).
// A mistyped field is a usage error: the body is refused at the door, one
// step before the codec would have refused it.
var settingsInputSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"name":{"type":"string"},"value":{"type":"string"},"section":{"type":"object"},"id":{"type":"string"},"force":{"type":"boolean"}}}`)

func settingsText(raw json.RawMessage, key string, required bool) (string, error) {
	if len(raw) == 0 {
		if required {
			return "", fmt.Errorf("args.%s required", key)
		}
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
	if required && strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("args.%s required", key)
	}
	return text, nil
}
func settingsBool(raw json.RawMessage, key string) (bool, error) {
	if len(raw) == 0 {
		return false, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, err
	}
	flag, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("args.%s must be a boolean", key)
	}
	return flag, nil
}
func Operations(reads func(context.Context) *Reads, writes func(context.Context) *Writes) ([]app.Operation, error) {
	if reads == nil || writes == nil {
		return nil, errors.New("settings read/write providers required")
	}
	spec := func(name string, read bool) opapi.Spec {
		return opapi.Spec{Name: name, ReadOnly: read, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: settingsInputSchema}
	}
	schema, err := app.NewOperation(spec(opapi.ConfigSchema, true), func(ctx context.Context, in SchemaInput) (SchemaOutput, error) { return reads(ctx).Schema(ctx, in) })
	if err != nil {
		return nil, err
	}
	values, err := app.NewOperation(spec(opapi.ConfigValues, true), func(ctx context.Context, in ValuesInput) (ValuesOutput, error) { return reads(ctx).Values(ctx, in) })
	if err != nil {
		return nil, err
	}
	// config_set carries its own request row: the setting's name and the value
	// under it. The row IS the operation's declared input — no schema is
	// written by hand here.
	set, err := app.NewOperation(opapi.Spec{Name: opapi.ConfigSet, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, in opapi.ConfigSetRequest) (SetOutput, error) {
		name, err := opapi.RequiredText(in.Name, "name")
		if err != nil {
			return SetOutput{}, err
		}
		name = strings.TrimSpace(name)
		return writes(ctx).Set(ctx, SetInput{Name: name, Value: in.Value})
	})
	if err != nil {
		return nil, err
	}
	register, err := app.NewOperation(spec(opapi.ConfigSchemaRegister, false), func(ctx context.Context, in RegisterRequest) (RegisterOutput, error) {
		if len(in.Section) == 0 {
			return RegisterOutput{}, errors.New("args.section required")
		}
		var sec Section
		if err := json.Unmarshal(in.Section, &sec); err != nil {
			return RegisterOutput{}, fmt.Errorf("decode section: %w", err)
		}
		return writes(ctx).Register(ctx, RegisterInput{Section: sec})
	})
	if err != nil {
		return nil, err
	}
	unregister, err := app.NewOperation(spec(opapi.ConfigSchemaUnregister, false), func(ctx context.Context, in UnregisterRequest) (UnregisterOutput, error) {
		id, err := settingsText(in.ID, "id", true)
		if err != nil {
			return UnregisterOutput{}, err
		}
		id = strings.TrimSpace(id)
		force, err := settingsBool(in.Force, "force")
		if err != nil {
			return UnregisterOutput{}, err
		}
		return writes(ctx).Unregister(ctx, UnregisterInput{ID: id, Force: force})
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{schema, values, set, register, unregister}, nil
}
