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
	sync, err := app.NewOperation(opapi.Spec{Name: "catalog_sync", AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "POST", Path: "/api/catalog/sync"}}, func(ctx context.Context, in SyncInput) (SyncOutput, error) { return provider(ctx).Sync(ctx, in) })
	if err != nil {
		return nil, err
	}
	list, err := app.NewOperation(opapi.Spec{Name: "catalog_list", ReadOnly: true, AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "GET", Path: "/api/catalog"}}, func(ctx context.Context, in ListInput) (ListOutput, error) { return provider(ctx).List(ctx, in) })
	if err != nil {
		return nil, err
	}
	discover, err := app.NewOperation(opapi.Spec{Name: "catalog_discover", AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "POST", Path: "/api/catalog/discover"}}, func(ctx context.Context, in DiscoverInput) (DiscoverOutput, error) {
		return provider(ctx).Discover(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{sync, list, discover}, nil
}
