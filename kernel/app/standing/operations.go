// SPDX-License-Identifier: MIT
package standing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	orders "github.com/agezt/agezt/kernel/standing"
	"strings"
)

// RequestInput keeps absent, null and wrong-typed fields distinct until each
// command applies its legacy validation order.
type RequestInput struct {
	ID          json.RawMessage `json:"id,omitempty"`
	Order       json.RawMessage `json:"order,omitempty"`
	Name        json.RawMessage `json:"name,omitempty"`
	Plan        json.RawMessage `json:"plan,omitempty"`
	Agent       json.RawMessage `json:"agent,omitempty"`
	Mode        json.RawMessage `json:"mode,omitempty"`
	MaxTrust    json.RawMessage `json:"max_trust,omitempty"`
	BriefingMin json.RawMessage `json:"briefing_min,omitempty"`
	Assure      json.RawMessage `json:"assure,omitempty"`
	Cooldown    json.RawMessage `json:"cooldown_sec,omitempty"`
	Enabled     json.RawMessage `json:"enabled,omitempty"`
}
type Providers struct {
	Service      func(context.Context) *Service
	Observations func(context.Context) *Observations
	Firing       func(context.Context) *Firing
}

var requestSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"id":{},"order":{},"name":{},"plan":{},"agent":{},"mode":{},"max_trust":{},"briefing_min":{},"assure":{},"cooldown_sec":{},"enabled":{}}}`)

func requestValue(raw json.RawMessage) any {
	var value any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &value)
	}
	return value
}
func requestText(raw json.RawMessage, key string) (TextField, error) {
	if len(raw) == 0 {
		return TextField{}, nil
	}
	value, ok := requestValue(raw).(string)
	if !ok {
		return TextField{Present: true}, fmt.Errorf("args.%s must be a string", key)
	}
	return TextField{Present: true, Value: value}, nil
}
func requestID(raw json.RawMessage) (string, error) {
	field, err := requestText(raw, "id")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(field.Value) == "" {
		return "", errors.New("args.id required")
	}
	return field.Value, nil
}
func requestNumber(raw json.RawMessage, key string) (NumberField, error) {
	if len(raw) == 0 {
		return NumberField{}, nil
	}
	value, ok := requestValue(raw).(float64)
	if !ok {
		return NumberField{Present: true}, fmt.Errorf("args.%s must be a number", key)
	}
	return NumberField{Present: true, Value: value}, nil
}
func requestEdit(in RequestInput) (EditInput, error) {
	id, err := requestID(in.ID)
	if err != nil {
		return EditInput{}, err
	}
	out := EditInput{ID: id}
	for _, field := range []struct {
		raw  json.RawMessage
		key  string
		dest *TextField
	}{{in.Name, "name", &out.Name}, {in.Plan, "plan", &out.Plan}, {in.Agent, "agent", &out.Agent}, {in.Mode, "mode", &out.Mode}, {in.MaxTrust, "max_trust", &out.MaxTrust}, {in.BriefingMin, "briefing_min", &out.BriefingMin}} {
		value, err := requestText(field.raw, field.key)
		if err != nil {
			return EditInput{}, err
		}
		*field.dest = value
	}
	out.Assure, err = requestNumber(in.Assure, "assure")
	if err != nil {
		return EditInput{}, err
	}
	out.Cooldown, err = requestNumber(in.Cooldown, "cooldown_sec")
	return out, err
}
func bind[O any](ops *[]app.Operation, name string, readOnly bool, handler func(context.Context, RequestInput) (O, error)) error {
	operation, err := app.NewOperation(opapi.Spec{Name: name, ReadOnly: readOnly, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: requestSchema}, handler)
	if err == nil {
		*ops = append(*ops, operation)
	}
	return err
}
func Operations(providers Providers) ([]app.Operation, error) {
	if providers.Service == nil || providers.Observations == nil || providers.Firing == nil {
		return nil, errors.New("standing service providers required")
	}
	var ops []app.Operation
	bindings := []func() error{
		func() error {
			return bind(&ops, "standing_list", true, func(ctx context.Context, _ RequestInput) (ListOutput, error) {
				return providers.Service(ctx).List(ctx, ListInput{})
			})
		},
		func() error {
			return bind(&ops, "standing_add", false, func(ctx context.Context, in RequestInput) (OrderOutput, error) {
				if len(in.Order) == 0 {
					return OrderOutput{}, errors.New("args.order required")
				}
				// Re-encode as the native JSON object transport did before typed binding.
				raw, err := json.Marshal(requestValue(in.Order))
				if err != nil {
					return OrderOutput{}, fmt.Errorf("args.order: %w", err)
				}
				var order orders.Order
				if err := json.Unmarshal(raw, &order); err != nil {
					return OrderOutput{}, fmt.Errorf("args.order: %w", err)
				}
				return providers.Service(ctx).Add(ctx, AddInput{Order: order})
			})
		},
		func() error {
			return bind(&ops, "standing_edit", false, func(ctx context.Context, in RequestInput) (EditOutput, error) {
				patch, err := requestEdit(in)
				if err != nil {
					return EditOutput{}, err
				}
				return providers.Service(ctx).Edit(ctx, patch)
			})
		},
		func() error {
			return bind(&ops, "standing_set_enabled", false, func(ctx context.Context, in RequestInput) (OrderOutput, error) {
				id, err := requestID(in.ID)
				if err != nil {
					return OrderOutput{}, err
				}
				// Native argBool reported presence for wrong types; all non-bools became false.
				enabled, _ := requestValue(in.Enabled).(bool)
				return providers.Service(ctx).SetEnabled(ctx, EnableInput{ID: id, Enabled: enabled})
			})
		},
		func() error {
			return bind(&ops, "standing_remove", false, func(ctx context.Context, in RequestInput) (RemoveOutput, error) {
				id, err := requestID(in.ID)
				if err != nil {
					return RemoveOutput{}, err
				}
				return providers.Service(ctx).Remove(ctx, RemoveInput{ID: id})
			})
		},
		func() error {
			return bind(&ops, "standing_why", true, func(ctx context.Context, in RequestInput) (WhyOutput, error) {
				id, err := requestID(in.ID)
				if err != nil {
					return WhyOutput{}, err
				}
				return providers.Observations(ctx).Why(ctx, WhyInput{ID: id})
			})
		},
		func() error {
			return bind(&ops, "standing_fire", false, func(ctx context.Context, in RequestInput) (FireOutput, error) {
				id, err := requestID(in.ID)
				if err != nil {
					return FireOutput{}, err
				}
				return providers.Firing(ctx).Fire(ctx, FireInput{ID: id})
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
