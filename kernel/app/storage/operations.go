// SPDX-License-Identifier: MIT

package storage

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("storage service provider required")
	}
	operation, err := app.NewOperation(opapi.Spec{Name: "storage_stats", ReadOnly: true, AllowUnknownInput: true}, func(ctx context.Context, in StatsInput) (StatsOutput, error) { return provider(ctx).Stats(ctx, in) })
	if err != nil {
		return nil, err
	}
	disk, err := app.NewOperation(opapi.Spec{Name: "disk_stats", ReadOnly: true, AllowUnknownInput: true}, func(ctx context.Context, in DiskInput) (DiskOutput, error) { return provider(ctx).Disk(ctx, in) })
	if err != nil {
		return nil, err
	}
	return []app.Operation{operation, disk}, nil
}
