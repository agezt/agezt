// SPDX-License-Identifier: MIT

package providers

import (
	"context"
	"errors"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

func OAuthOperations(provider func(context.Context) *OAuth) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("OAuth state provider required")
	}
	start, err := app.NewOperation(opapi.Spec{Name: "provider_oauth_start", AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "POST", Path: "/api/provider/oauth/start"}}, func(ctx context.Context, in OAuthStartInput) (OAuthStartOutput, error) {
		return provider(ctx).Start(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	status, err := app.NewOperation(opapi.Spec{Name: "provider_oauth_status", ReadOnly: true, AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "POST", Path: "/api/provider/oauth/status"}}, func(ctx context.Context, in OAuthStatusInput) (OAuthStatusOutput, error) {
		return provider(ctx).Status(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	imported, err := app.NewOperation(opapi.Spec{Name: "provider_oauth_import", AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "POST", Path: "/api/provider/oauth/import"}}, func(ctx context.Context, in OAuthImportInput) (OAuthImportOutput, error) {
		return provider(ctx).Import(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	logout, err := app.NewOperation(opapi.Spec{Name: "provider_oauth_logout", AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "POST", Path: "/api/provider/oauth/logout"}}, func(ctx context.Context, in OAuthLogoutInput) (OAuthLogoutOutput, error) {
		return provider(ctx).Logout(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{start, status, imported, logout}, nil
}
