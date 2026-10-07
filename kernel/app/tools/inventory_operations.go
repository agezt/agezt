// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

// InventoryOperations binds the primary aggregate read. Unknown arguments retain
// the native no-argument command's existing compatibility.
func InventoryOperations(provider func(context.Context) *Inventory) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("tool inventory provider required")
	}
	operation, err := app.NewOperation(opapi.Spec{Name: "tool_list", ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, in InventoryInput) (InventoryOutput, error) {
		return provider(ctx).List(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{operation}, nil
}
