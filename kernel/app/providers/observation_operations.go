// SPDX-License-Identifier: MIT

package providers

import (
	"context"
	"errors"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

type ObservationPageInput struct {
	Limit   *float64 `json:"limit,omitempty"`
	SinceMS float64  `json:"since_ms,omitempty"`
}

type ObservationLogInput struct {
	ObservationPageInput
	Cursor    any  `json:"cursor,omitempty"`
	Fallbacks bool `json:"fallbacks,omitempty"`
}

type ObservationStatsInput struct {
	SinceMS float64 `json:"since_ms,omitempty"`
}

func observationWindow(since float64) ObservationInput {
	window := int64(since)
	var cutoff int64
	if window > 0 {
		cutoff = time.Now().UnixMilli() - window
	}
	return ObservationInput{WindowMS: window, CutoffMS: cutoff}
}

func observationPage(in ObservationPageInput) ObservationInput {
	bounded := observationWindow(in.SinceMS)
	bounded.Limit = 20
	if in.Limit != nil {
		bounded.Limit = int(*in.Limit)
	}
	if bounded.Limit < 1 {
		bounded.Limit = 1
	}
	if bounded.Limit > 1000 {
		bounded.Limit = 1000
	}
	return bounded
}

func ObservationOperations(provider func(context.Context) *Observations) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("provider observation service provider required")
	}
	log, err := app.NewOperation(opapi.Spec{Name: "provider_log", ReadOnly: true, Authz: opapi.OwnTenant, Tenancy: opapi.CallerTenant, AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "GET", Path: "/api/provider_log"}}, func(ctx context.Context, in ObservationLogInput) (ObservationLogOutput, error) {
		bounded := observationPage(in.ObservationPageInput)
		bounded.Cursor, bounded.FallbacksOnly = in.Cursor, in.Fallbacks
		return provider(ctx).Log(bounded)
	})
	if err != nil {
		return nil, err
	}
	stats, err := app.NewOperation(opapi.Spec{Name: "provider_stats", ReadOnly: true, Authz: opapi.OwnTenant, Tenancy: opapi.CallerTenant, AllowUnknownInput: true}, func(ctx context.Context, in ObservationStatsInput) (ObservationStatsOutput, error) {
		return provider(ctx).Stats(observationWindow(in.SinceMS))
	})
	if err != nil {
		return nil, err
	}
	rejections, err := app.NewOperation(opapi.Spec{Name: "provider_rejections", ReadOnly: true, Authz: opapi.OwnTenant, Tenancy: opapi.CallerTenant, AllowUnknownInput: true}, func(ctx context.Context, in ObservationPageInput) (ObservationRejectionsOutput, error) {
		return provider(ctx).Rejections(observationPage(in))
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{log, stats, rejections}, nil
}
