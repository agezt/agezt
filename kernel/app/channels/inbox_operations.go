// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

type InboxRequest struct {
	Limit   json.RawMessage `json:"limit,omitempty"`
	Channel json.RawMessage `json:"channel,omitempty"`
	Cursor  json.RawMessage `json:"cursor,omitempty"`
}

func (r InboxRequest) input() InboxInput {
	in := InboxInput{}
	var value any
	if json.Unmarshal(r.Limit, &value) == nil {
		if number, ok := value.(float64); ok {
			limit := int(number)
			in.Limit = &limit
		}
	}
	value = nil
	if json.Unmarshal(r.Channel, &value) == nil {
		in.Channel, _ = value.(string)
	}
	if len(r.Cursor) > 0 {
		value = nil
		if err := json.Unmarshal(r.Cursor, &value); err != nil {
			in.CursorError = err
		} else {
			text, ok := value.(string)
			if ok {
				in.Cursor = text
			} else {
				in.CursorError = fmt.Errorf("args.cursor must be a string")
			}
		}
	}
	return in
}

func InboxOperations(provider func(context.Context) *Inbox) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("channel inbox service provider required")
	}
	op, err := app.NewOperation(opapi.Spec{Name: "inbox", ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true,
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"limit":{},"channel":{},"cursor":{}}}`),
		HTTP:        opapi.HTTP{Method: "GET", Path: "/api/inbox"}}, func(ctx context.Context, in InboxRequest) (InboxOutput, error) {
		return provider(ctx).List(ctx, in.input())
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
