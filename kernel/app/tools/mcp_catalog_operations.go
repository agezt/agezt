// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

func MCPCatalogOperations(provider func(context.Context) *MCPCatalog) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("MCP catalog provider required")
	}
	operation, err := app.NewOperation(opapi.Spec{Name: "mcp_list", ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, in MCPListInput) (MCPListOutput, error) { return provider(ctx).List(ctx, in) })
	if err != nil {
		return nil, err
	}
	return []app.Operation{operation}, nil
}
