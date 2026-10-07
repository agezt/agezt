// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

// Raw fields keep native argument type errors in their historical order, after
// writer audit admission and before the service provider can access stores.
type SetAccountRequest struct {
	Kind  json.RawMessage `json:"kind,omitempty"`
	Label json.RawMessage `json:"label,omitempty"`
	Name  json.RawMessage `json:"name,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}
type RemoveAccountRequest struct {
	Kind  json.RawMessage `json:"kind,omitempty"`
	Label json.RawMessage `json:"label,omitempty"`
}

func accountText(raw json.RawMessage, key string) (string, error) {
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

func AccountOperations(provider func(context.Context) *Accounts) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("channel account service provider required")
	}
	set, err := app.NewOperation(opapi.Spec{Name: "channel_account_set", Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true,
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"kind":{},"label":{},"name":{},"value":{}}}`),
		HTTP:        opapi.HTTP{Method: "POST", Path: "/api/channel/account/set"}}, func(ctx context.Context, in SetAccountRequest) (SetAccountOutput, error) {
		kind, err := accountText(in.Kind, "kind")
		if err != nil {
			return SetAccountOutput{}, err
		}
		label, err := accountText(in.Label, "label")
		if err != nil {
			return SetAccountOutput{}, err
		}
		name, err := accountText(in.Name, "name")
		if err != nil {
			return SetAccountOutput{}, err
		}
		value, err := accountText(in.Value, "value")
		if err != nil {
			return SetAccountOutput{}, err
		}
		return provider(ctx).Set(ctx, SetAccountInput{Kind: kind, Label: label, Name: name, Value: value})
	})
	if err != nil {
		return nil, err
	}
	remove, err := app.NewOperation(opapi.Spec{Name: "channel_account_remove", Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true,
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"kind":{},"label":{}}}`),
		HTTP:        opapi.HTTP{Method: "POST", Path: "/api/channel/account/remove"}}, func(ctx context.Context, in RemoveAccountRequest) (RemoveAccountOutput, error) {
		kind, err := accountText(in.Kind, "kind")
		if err != nil {
			return RemoveAccountOutput{}, err
		}
		label, err := accountText(in.Label, "label")
		if err != nil {
			return RemoveAccountOutput{}, err
		}
		return provider(ctx).Remove(ctx, RemoveAccountInput{Kind: kind, Label: label})
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{set, remove}, nil
}
