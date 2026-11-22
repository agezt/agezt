// SPDX-License-Identifier: MIT

package catalog

import (
	"context"
	"errors"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("catalog service provider required")
	}
	sync, err := app.NewOperation(opapi.Spec{Name: opapi.CatalogSync, AllowUnknownInput: true}, func(ctx context.Context, in SyncInput) (SyncOutput, error) { return provider(ctx).Sync(ctx, in) })
	if err != nil {
		return nil, err
	}
	list, err := app.NewOperation(opapi.Spec{Name: opapi.CatalogList, ReadOnly: true, AllowUnknownInput: true}, func(ctx context.Context, in ListInput) (ListOutput, error) { return provider(ctx).List(ctx, in) })
	if err != nil {
		return nil, err
	}
	discover, err := app.NewOperation(opapi.Spec{Name: opapi.CatalogDiscover, AllowUnknownInput: true}, func(ctx context.Context, in DiscoverInput) (DiscoverOutput, error) {
		return provider(ctx).Discover(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{sync, list, discover}, nil
}
