// SPDX-License-Identifier: MIT
package config

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("config read provider required")
	}
	operation, err := app.NewOperation(opapi.Spec{Name: "config", ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, in ShowInput) (ShowOutput, error) { return provider(ctx).Show(ctx, in) })
	if err != nil {
		return nil, err
	}
	return []app.Operation{operation}, nil
}
