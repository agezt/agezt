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

type SetRequest struct {
	Name  json.RawMessage `json:"name,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}
type RegisterRequest struct {
	Section json.RawMessage `json:"section,omitempty"`
}
type UnregisterRequest struct {
	ID    json.RawMessage `json:"id,omitempty"`
	Force json.RawMessage `json:"force,omitempty"`
}

var settingsInputSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"name":{},"value":{},"section":{},"id":{},"force":{}}}`)

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
	schema, err := app.NewOperation(spec("config_schema", true), func(ctx context.Context, in SchemaInput) (SchemaOutput, error) { return reads(ctx).Schema(ctx, in) })
	if err != nil {
		return nil, err
	}
	values, err := app.NewOperation(spec("config_values", true), func(ctx context.Context, in ValuesInput) (ValuesOutput, error) { return reads(ctx).Values(ctx, in) })
	if err != nil {
		return nil, err
	}
	set, err := app.NewOperation(spec("config_set", false), func(ctx context.Context, in SetRequest) (SetOutput, error) {
		name, err := settingsText(in.Name, "name", true)
		if err != nil {
			return SetOutput{}, err
		}
		name = strings.TrimSpace(name)
		value, err := settingsText(in.Value, "value", false)
		if err != nil {
			return SetOutput{}, err
		}
		return writes(ctx).Set(ctx, SetInput{Name: name, Value: value})
	})
	if err != nil {
		return nil, err
	}
	register, err := app.NewOperation(spec("config_schema_register", false), func(ctx context.Context, in RegisterRequest) (RegisterOutput, error) {
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
	unregister, err := app.NewOperation(spec("config_schema_unregister", false), func(ctx context.Context, in UnregisterRequest) (UnregisterOutput, error) {
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
