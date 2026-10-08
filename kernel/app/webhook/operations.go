// SPDX-License-Identifier: MIT
package webhook

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

type LogRequest struct {
	Failed  json.RawMessage `json:"failed,omitempty"`
	Limit   json.RawMessage `json:"limit,omitempty"`
	SinceMS json.RawMessage `json:"since_ms,omitempty"`
	Cursor  json.RawMessage `json:"cursor,omitempty"`
}
type StatsRequest struct {
	SinceMS json.RawMessage `json:"since_ms,omitempty"`
}

func value(raw json.RawMessage) any     { var out any; _ = json.Unmarshal(raw, &out); return out }
func numeric(raw json.RawMessage) int64 { n, _ := value(raw).(float64); return int64(n) }

func Operations(provider func(context.Context) *Observability) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("webhook observability provider required")
	}
	log, err := app.NewOperation(opapi.Spec{Name: "webhook_log", ReadOnly: true, Authz: opapi.OwnTenant, Tenancy: opapi.CallerTenant, AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "GET", Path: "/api/webhook_log"}, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"failed":{},"limit":{},"since_ms":{},"cursor":{}}}`)}, func(ctx context.Context, in LogRequest) (LogOutput, error) {
		failed := false
		if len(in.Failed) > 0 {
			var ok bool
			failed, ok = value(in.Failed).(bool)
			if !ok {
				return LogOutput{}, errors.New("args.failed must be a boolean")
			}
		}
		var limit *int
		if number, ok := value(in.Limit).(float64); ok {
			n := int(number)
			limit = &n
		}
		return provider(ctx).Log(ctx, LogInput{Limit: limit, SinceMS: numeric(in.SinceMS), Cursor: value(in.Cursor), FailedOnly: failed})
	})
	if err != nil {
		return nil, err
	}
	stats, err := app.NewOperation(opapi.Spec{Name: "webhook_stats", ReadOnly: true, Authz: opapi.OwnTenant, Tenancy: opapi.CallerTenant, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"since_ms":{}}}`)}, func(ctx context.Context, in StatsRequest) (StatsOutput, error) {
		return provider(ctx).Stats(ctx, numeric(in.SinceMS))
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{log, stats}, nil
}
