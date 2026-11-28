// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

// listInput reads the listing's page size and cursor from the request row.
//
// The listing's page size is the caller's — a count above zero, a fraction
// included, which truncates to no rows at all — and it is capped at the
// family's own cap of 1000.
func listInput(in opapi.AgentListRequest) ListInput {
	out := ListInput{}
	if in.Limit.Present() && in.Limit.Number() > 0 {
		out.Limit = int(in.Limit.Number())
	}
	out.Cursor = in.Cursor
	return out
}
func ListOperations(provider func(context.Context) *ListService) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("roster list provider required")
	}
	output, err := listOutputSchema()
	if err != nil {
		return nil, err
	}
	op, err := app.NewOperation(opapi.Spec{Name: opapi.AgentList, ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, OutputSchema: output}, func(ctx context.Context, in opapi.AgentListRequest) (ListOutput, error) {
		return provider(ctx).List(ctx, listInput(in))
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
