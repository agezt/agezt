// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/toolbox"
)

func ToolboxReadOperations(provider func(context.Context) *ToolboxReads) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("toolbox read provider required")
	}
	spec := func(name string) opapi.Spec {
		return opapi.Spec{Name: name, ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}
	}
	detect, err := app.NewOperation(spec("toolbox_detect"), func(ctx context.Context, in ToolboxDetectInput) (toolbox.Inventory, error) {
		return provider(ctx).Detect(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	outdated, err := app.NewOperation(spec("toolbox_outdated"), func(ctx context.Context, in ToolboxOutdatedInput) (ToolboxOutdatedOutput, error) {
		return provider(ctx).Outdated(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{detect, outdated}, nil
}
