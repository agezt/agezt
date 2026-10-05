// SPDX-License-Identifier: MIT

package providers

import (
	"context"
	"errors"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

func ProbeOperations(provider func(context.Context) *Probe) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("provider probe service provider required")
	}
	op, err := app.NewOperation(opapi.Spec{Name: "provider_probe", ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "POST", Path: "/api/provider/probe"}}, func(ctx context.Context, in ProbeInput) (ProbeOutput, error) {
		return provider(ctx).Check(in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
