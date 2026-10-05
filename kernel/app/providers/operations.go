// SPDX-License-Identifier: MIT

package providers

import (
	"context"
	"errors"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

type KeyTargetInput struct {
	Provider string `json:"provider,omitempty"`
	Env      string `json:"env,omitempty"`
}
type KeyLabelInput struct {
	KeyTargetInput
	Label string `json:"label,omitempty"`
}
type KeyAddInput struct {
	KeyLabelInput
	Value  string `json:"value,omitempty"`
	Active bool   `json:"active,omitempty"`
}

func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("provider service provider required")
	}
	connect, err := app.NewOperation(opapi.Spec{Name: "provider_connect", AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "POST", Path: "/api/provider/connect"}}, func(ctx context.Context, in ConnectInput) (ConnectOutput, error) {
		return provider(ctx).Connect(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	reload, err := app.NewOperation(opapi.Spec{Name: "provider_reload", AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "POST", Path: "/api/provider/reload"}}, func(ctx context.Context, in ReloadInput) (ReloadOutput, error) { return provider(ctx).Reload(ctx, in) })
	if err != nil {
		return nil, err
	}
	list, err := app.NewOperation(opapi.Spec{Name: "provider_key_list", ReadOnly: true, AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "GET", Path: "/api/provider/keys"}}, func(ctx context.Context, in KeyTargetInput) (KeyListOutput, error) {
		return provider(ctx).KeyList(ctx, KeyInput{Provider: in.Provider, Env: in.Env})
	})
	if err != nil {
		return nil, err
	}
	add, err := app.NewOperation(opapi.Spec{Name: "provider_key_add", AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "POST", Path: "/api/provider/keys/add"}}, func(ctx context.Context, in KeyAddInput) (KeyAddOutput, error) {
		return provider(ctx).KeyAdd(ctx, KeyInput{Provider: in.Provider, Env: in.Env, Label: in.Label, Value: in.Value, Active: in.Active})
	})
	if err != nil {
		return nil, err
	}
	activate, err := app.NewOperation(opapi.Spec{Name: "provider_key_activate", AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "POST", Path: "/api/provider/keys/activate"}}, func(ctx context.Context, in KeyLabelInput) (KeyActivateOutput, error) {
		return provider(ctx).KeyActivate(ctx, KeyInput{Provider: in.Provider, Env: in.Env, Label: in.Label})
	})
	if err != nil {
		return nil, err
	}
	remove, err := app.NewOperation(opapi.Spec{Name: "provider_key_remove", AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "POST", Path: "/api/provider/keys/remove"}}, func(ctx context.Context, in KeyLabelInput) (KeyRemoveOutput, error) {
		return provider(ctx).KeyRemove(ctx, KeyInput{Provider: in.Provider, Env: in.Env, Label: in.Label})
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{connect, reload, list, add, activate, remove}, nil
}
