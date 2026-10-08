// SPDX-License-Identifier: MIT
package roster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

type ListRequest struct {
	Limit  json.RawMessage `json:"limit,omitempty"`
	Cursor json.RawMessage `json:"cursor,omitempty"`
}

func (r ListRequest) input() ListInput {
	var in ListInput
	if len(r.Limit) > 0 {
		var v any
		_ = json.Unmarshal(r.Limit, &v)
		number, ok := v.(float64)
		if !ok {
			in.DecodeError = fmt.Errorf("args.limit must be a number")
			return in
		}
		if number > 0 {
			in.Limit = int(number)
		}
	}
	if len(r.Cursor) > 0 {
		var v any
		_ = json.Unmarshal(r.Cursor, &v)
		cursor, ok := v.(string)
		if !ok {
			in.DecodeError = fmt.Errorf("args.cursor must be a string")
		} else {
			in.Cursor = cursor
		}
	}
	return in
}
func ListOperations(provider func(context.Context) *ListService) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("roster list provider required")
	}
	output, err := listOutputSchema()
	if err != nil {
		return nil, err
	}
	op, err := app.NewOperation(opapi.Spec{Name: "agent_list", ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"limit":{},"cursor":{}}}`), OutputSchema: output, HTTP: opapi.HTTP{Method: "GET", Path: "/api/agents"}}, func(ctx context.Context, in ListRequest) (ListOutput, error) {
		return provider(ctx).List(ctx, in.input())
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
