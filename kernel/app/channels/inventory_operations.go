// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"errors"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

func InventoryOperations(provider func(context.Context) *Inventory) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("channel inventory service provider required")
	}
	operation, err := app.NewOperation(opapi.Spec{Name: "channel_list", ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "GET", Path: "/api/channels"}}, func(ctx context.Context, in ListInput) (ListOutput, error) { return provider(ctx).List(ctx, in) })
	if err != nil {
		return nil, err
	}
	return []app.Operation{operation}, nil
}
