// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

func ACPInventoryOperations(provider func(context.Context) *ACPInventory) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("ACP inventory service provider required")
	}
	operation, err := app.NewOperation(opapi.Spec{Name: "acp_agents", ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "GET", Path: "/api/acp/agents"}}, func(ctx context.Context, in ACPInput) (ACPOutput, error) { return provider(ctx).List(ctx, in) })
	if err != nil {
		return nil, err
	}
	return []app.Operation{operation}, nil
}
