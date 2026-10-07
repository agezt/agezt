// SPDX-License-Identifier: MIT
package plugins

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("plugin inventory provider required")
	}
	operation, err := app.NewOperation(opapi.Spec{Name: "plugin_list", ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, in ListInput) (ListOutput, error) { return provider(ctx).List(ctx, in) })
	if err != nil {
		return nil, err
	}
	return []app.Operation{operation}, nil
}
