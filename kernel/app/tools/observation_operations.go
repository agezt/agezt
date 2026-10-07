// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/journalview"
	"time"
)

type ObservationRequest struct {
	Errors  json.RawMessage `json:"errors,omitempty"`
	Tool    json.RawMessage `json:"tool,omitempty"`
	SlowMS  json.RawMessage `json:"slow_ms,omitempty"`
	Limit   json.RawMessage `json:"limit,omitempty"`
	SinceMS json.RawMessage `json:"since_ms,omitempty"`
	Cursor  json.RawMessage `json:"cursor,omitempty"`
}

var observationSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"errors":{},"tool":{},"slow_ms":{},"limit":{},"since_ms":{},"cursor":{}}}`)

func observationValue(raw json.RawMessage) any {
	var value any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &value)
	}
	return value
}
func observationNumber(raw json.RawMessage) int64 {
	value, _ := observationValue(raw).(float64)
	return int64(value)
}
func observationText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	value, ok := observationValue(raw).(string)
	if !ok {
		return "", errors.New("args.tool must be a string")
	}
	return value, nil
}
func observationBool(raw json.RawMessage) (bool, error) {
	if len(raw) == 0 {
		return false, nil
	}
	value, ok := observationValue(raw).(bool)
	if !ok {
		return false, errors.New("args.errors must be a boolean")
	}
	return value, nil
}
func observationCutoff(raw json.RawMessage, now func() int64) int64 {
	since := observationNumber(raw)
	if since <= 0 {
		return 0
	}
	return now() - since
}
func observationLimit(raw json.RawMessage) int {
	limit := 20
	if number, ok := observationValue(raw).(float64); ok {
		limit = int(number)
	}
	if limit < 1 {
		return 1
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}
func ObservationOperations(provider func(context.Context) *Observations, now func() int64) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("tool observation provider required")
	}
	if now == nil {
		now = func() int64 { return time.Now().UnixMilli() }
	}
	spec := func(name string) opapi.Spec {
		return opapi.Spec{Name: name, ReadOnly: true, Authz: opapi.OwnTenant, Tenancy: opapi.CallerTenant, AllowUnknownInput: true, InputSchema: observationSchema}
	}
	log, err := app.NewOperation(spec("tool_log"), func(ctx context.Context, in ObservationRequest) (LogOutput, error) {
		errorsOnly, err := observationBool(in.Errors)
		if err != nil {
			return LogOutput{}, err
		}
		tool, err := observationText(in.Tool)
		if err != nil {
			return LogOutput{}, err
		}
		return provider(ctx).Log(ctx, LogInput{ErrorsOnly: errorsOnly, Tool: tool, SlowMS: observationNumber(in.SlowMS), Page: journalview.Input{Limit: observationLimit(in.Limit), CutoffMS: observationCutoff(in.SinceMS, now), Cursor: observationValue(in.Cursor)}})
	})
	if err != nil {
		return nil, fmt.Errorf("tool log binding: %w", err)
	}
	stats, err := app.NewOperation(spec("tool_stats"), func(ctx context.Context, in ObservationRequest) (StatsOutput, error) {
		tool, err := observationText(in.Tool)
		if err != nil {
			return StatsOutput{}, err
		}
		return provider(ctx).Stats(ctx, StatsInput{Tool: tool, CutoffMS: observationCutoff(in.SinceMS, now), WindowMS: observationNumber(in.SinceMS)})
	})
	if err != nil {
		return nil, fmt.Errorf("tool stats binding: %w", err)
	}
	return []app.Operation{log, stats}, nil
}
