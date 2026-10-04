// SPDX-License-Identifier: MIT

package system

import (
	"context"
	"errors"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

// Operations owns pilot metadata and binds fresh host state at handler entry.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("system service provider required")
	}
	status, err := app.NewOperation(opapi.Spec{Name: "status", ReadOnly: true, AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "GET", Path: "/api/status"}}, func(ctx context.Context, input StatusInput) (StatusOutput, error) {
		return provider(ctx).Status(ctx, input)
	})
	if err != nil {
		return nil, err
	}
	version, err := app.NewOperation(opapi.Spec{Name: "version", ReadOnly: true, AllowUnknownInput: true, OutputSchema: versionOutputSchema(), HTTP: opapi.HTTP{Method: "GET", Path: "/api/version"}}, func(ctx context.Context, input VersionInput) (VersionOutput, error) {
		return provider(ctx).Version(ctx, input)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{status, version}, nil
}
