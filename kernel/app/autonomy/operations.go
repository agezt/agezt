// SPDX-License-Identifier: MIT
package autonomy

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

// FeedRequest preserves the native numeric-only limit convention. Unknown and
// wrong-type arguments retain the default rather than becoming new schema errors.
type FeedRequest struct {
	Limit json.RawMessage `json:"limit,omitempty"`
}

var feedRequestSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"limit":{}}}`)

func nativeFeedLimit(raw json.RawMessage) int {
	var value any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &value)
	}
	if limit, ok := value.(float64); ok {
		return int(limit)
	}
	return FeedDefaultLimit
}
func Operations(provider func(context.Context) *Feed) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("autonomy feed provider required")
	}
	operation, err := app.NewOperation(opapi.Spec{Name: "autonomy_feed", ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: feedRequestSchema}, func(ctx context.Context, in FeedRequest) (FeedOutput, error) {
		return provider(ctx).List(ctx, FeedInput{Limit: nativeFeedLimit(in.Limit)})
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{operation}, nil
}
