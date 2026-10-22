// SPDX-License-Identifier: MIT

// Package reaper owns the on-demand reaper scan: the read-only detail behind
// the pulse reaper observer. It detects dead, degraded, misconfigured and
// routing-troubled agents and stale artifacts; the operator still retires or
// collects.
package reaper

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/runtime"
)

// DefaultDays is the idle and stale grace period when none (or a non-positive
// one) is requested.
const DefaultDays = 30

// Service scans the primary kernel at the daemon's clock.
type Service struct {
	scan func(agentIdleCutoffMs, artifactStaleCutoffMs int64) runtime.ReaperReport
	now  func() time.Time
}

func New(scan func(int64, int64) runtime.ReaperReport, now func() time.Time) *Service {
	return &Service{scan: scan, now: now}
}

type ScanRequest struct {
	IdleDays  json.RawMessage `json:"idle_days,omitempty"`
	StaleDays json.RawMessage `json:"stale_days,omitempty"`
}

// The scan's rows are the runtime's finding rows — one declaration per
// finding, published as they are. The application does not rebuild them field
// by field any more.
type (
	DeadRow          = runtime.ReaperAgent
	DegradedRow      = runtime.DegradedAgent
	MisconfiguredRow = runtime.MisconfiguredAgent
	RetryRow         = runtime.RetryPressureAgent
	RoutingRow       = runtime.RoutingPressureAgent
	ForcedRow        = runtime.RoutingForcedAgent
	UnstableRow      = runtime.RoutingUnstableAgent
)

// ScanOutput lists every finding with its count; every list is an array and
// every row carries all of its fields.
type ScanOutput struct {
	DeadAgents            []DeadRow          `json:"dead_agents"`
	DeadCount             int                `json:"dead_count"`
	DegradedAgents        []DegradedRow      `json:"degraded_agents"`
	DegradedCount         int                `json:"degraded_count"`
	MisconfiguredAgents   []MisconfiguredRow `json:"misconfigured_agents"`
	MisconfiguredCount    int                `json:"misconfigured_count"`
	RetryPressureAgents   []RetryRow         `json:"retry_pressure_agents"`
	RetryPressureCount    int                `json:"retry_pressure_count"`
	RoutingPressureAgents []RoutingRow       `json:"routing_pressure_agents"`
	RoutingPressureCount  int                `json:"routing_pressure_count"`
	ForcedProbationAgents []ForcedRow        `json:"routing_forced_probation_agents"`
	ForcedProbationCount  int                `json:"routing_forced_probation_count"`
	ForcedFailedAgents    []ForcedRow        `json:"routing_forced_failed_agents"`
	ForcedFailedCount     int                `json:"routing_forced_failed_count"`
	ForcedExhaustedAgents []ForcedRow        `json:"routing_forced_exhausted_agents"`
	ForcedExhaustedCount  int                `json:"routing_forced_exhausted_count"`
	RoutingUnstableAgents []UnstableRow      `json:"routing_unstable_agents"`
	RoutingUnstableCount  int                `json:"routing_unstable_count"`
	StaleArtifacts        int                `json:"stale_artifacts"`
	StaleBytes            int64              `json:"stale_bytes"`
	IdleDays              int                `json:"idle_days"`
	StaleDays             int                `json:"stale_days"`
}

// days reads a lenient day count: a JSON number truncates toward zero, and
// anything else, or a result below one, is the default.
func days(raw json.RawMessage) int {
	n := DefaultDays
	if len(raw) > 0 {
		var v any
		_ = json.Unmarshal(raw, &v)
		if f, ok := v.(float64); ok {
			n = int(f)
		}
	}
	if n < 1 {
		n = DefaultDays
	}
	return n
}

// copyRows hands the scan's rows over as they are; the copy keeps the
// output's lists empty arrays (never null) without the app knowing the row's
// fields.
func copyRows[T any](rows []T) []T {
	out := make([]T, len(rows))
	copy(out, rows)
	return out
}

// Scan judges agents idle for idle_days and artifacts older than stale_days,
// both measured back from the daemon's clock.
func (s *Service) Scan(_ context.Context, in ScanRequest) (ScanOutput, error) {
	idleDays, staleDays := days(in.IdleDays), days(in.StaleDays)
	now := s.now()
	rep := s.scan(now.Add(-time.Duration(idleDays)*24*time.Hour).UnixMilli(), now.Add(-time.Duration(staleDays)*24*time.Hour).UnixMilli())
	out := ScanOutput{
		DeadAgents:            copyRows(rep.DeadAgents),
		DegradedAgents:        copyRows(rep.DegradedAgents),
		MisconfiguredAgents:   copyRows(rep.MisconfiguredAgents),
		RetryPressureAgents:   copyRows(rep.RetryPressure),
		RoutingPressureAgents: copyRows(rep.RoutingPressure),
		ForcedProbationAgents: copyRows(rep.RoutingForced),
		ForcedFailedAgents:    copyRows(rep.RoutingForcedFailed),
		ForcedExhaustedAgents: copyRows(rep.RoutingForcedExhausted),
		RoutingUnstableAgents: copyRows(rep.RoutingUnstable),
		StaleArtifacts:        rep.StaleArtifacts,
		StaleBytes:            rep.StaleBytes,
		IdleDays:              idleDays,
		StaleDays:             staleDays,
	}
	out.DeadCount, out.DegradedCount, out.MisconfiguredCount = len(out.DeadAgents), len(out.DegradedAgents), len(out.MisconfiguredAgents)
	out.RetryPressureCount, out.RoutingPressureCount = len(out.RetryPressureAgents), len(out.RoutingPressureAgents)
	out.ForcedProbationCount, out.ForcedFailedCount, out.ForcedExhaustedCount = len(out.ForcedProbationAgents), len(out.ForcedFailedAgents), len(out.ForcedExhaustedAgents)
	out.RoutingUnstableCount = len(out.RoutingUnstableAgents)
	return out, nil
}

// Operations declares the unaudited, operator-only scan on the Web UI's route.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("reaper provider required")
	}
	output, err := schema.FromType(reflect.TypeFor[ScanOutput](), false)
	if err != nil {
		return nil, err
	}
	op, err := app.NewOperation(opapi.Spec{Name: "reaper_scan", ReadOnly: true, OutputSchema: output, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"idle_days":{},"stale_days":{}}}`), HTTP: opapi.HTTP{Method: "GET", Path: "/api/reaper/scan"}}, func(ctx context.Context, in ScanRequest) (ScanOutput, error) {
		return provider(ctx).Scan(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
