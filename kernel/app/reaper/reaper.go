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

type DeadRow struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	LastActiveMS int64  `json:"last_active_ms"`
}

type DegradedRow struct {
	Slug              string `json:"slug"`
	Name              string `json:"name"`
	Failures          int    `json:"failures"`
	Window            int    `json:"window"`
	Threshold         int    `json:"threshold"`
	DoctorAgent       string `json:"doctor_agent"`
	SelfRepairEnabled bool   `json:"self_repair_enabled"`
	EscalateTo        string `json:"escalate_to"`
	LastFailureMS     int64  `json:"last_failure_ms"`
	LastReason        string `json:"last_reason"`
}

type MisconfiguredRow struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name"`
	Issues            []string `json:"issues"`
	DoctorAgent       string   `json:"doctor_agent"`
	SelfRepairEnabled bool     `json:"self_repair_enabled"`
	EscalateTo        string   `json:"escalate_to"`
}

type RetryRow struct {
	Slug              string `json:"slug"`
	Name              string `json:"name"`
	Count             int    `json:"count"`
	Threshold         int    `json:"threshold"`
	WindowSec         int    `json:"window_sec"`
	DoctorAgent       string `json:"doctor_agent"`
	SelfRepairEnabled bool   `json:"self_repair_enabled"`
	EscalateTo        string `json:"escalate_to"`
	LastRetryMS       int64  `json:"last_retry_ms"`
	LastReason        string `json:"last_reason"`
	NextAttempt       int    `json:"next_attempt"`
	MaxAttempts       int    `json:"max_attempts"`
}

type RoutingRow struct {
	Slug              string `json:"slug"`
	Name              string `json:"name"`
	Count             int    `json:"count"`
	Threshold         int    `json:"threshold"`
	WindowSec         int    `json:"window_sec"`
	DoctorAgent       string `json:"doctor_agent"`
	SelfRepairEnabled bool   `json:"self_repair_enabled"`
	EscalateTo        string `json:"escalate_to"`
	LastFallbackMS    int64  `json:"last_fallback_ms"`
	LastReason        string `json:"last_reason"`
	LastFailedModel   string `json:"last_failed_model"`
	LastNextModel     string `json:"last_next_model"`
	TaskType          string `json:"task_type"`
}

// ForcedRow is one agent on a forced routing chain: in probation, failed after
// it, or exhausted by repeated forced generations.
type ForcedRow struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name"`
	Count             int      `json:"count"`
	Threshold         int      `json:"threshold"`
	WindowSec         int      `json:"window_sec"`
	DoctorAgent       string   `json:"doctor_agent"`
	SelfRepairEnabled bool     `json:"self_repair_enabled"`
	EscalateTo        string   `json:"escalate_to"`
	LastFallbackMS    int64    `json:"last_fallback_ms"`
	LastForcedMS      int64    `json:"last_forced_ms"`
	LastReason        string   `json:"last_reason"`
	TaskType          string   `json:"task_type"`
	ForcedChain       []string `json:"forced_chain"`
	ForceGeneration   int      `json:"routing_force_generation"`
}

type UnstableRow struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name"`
	Count             int      `json:"count"`
	Threshold         int      `json:"threshold"`
	WindowSec         int      `json:"window_sec"`
	DoctorAgent       string   `json:"doctor_agent"`
	SelfRepairEnabled bool     `json:"self_repair_enabled"`
	EscalateTo        string   `json:"escalate_to"`
	LastRollbackMS    int64    `json:"last_rollback_ms"`
	TaskType          string   `json:"task_type"`
	CurrentChain      []string `json:"current_chain"`
	PreviousChain     []string `json:"previous_chain"`
	LastReason        string   `json:"last_reason"`
}

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

func forced(rows []runtime.RoutingForcedProbationAgent) []ForcedRow {
	out := make([]ForcedRow, 0, len(rows))
	for _, a := range rows {
		out = append(out, ForcedRow{Slug: a.Slug, Name: a.Name, Count: a.Count, Threshold: a.Threshold, WindowSec: a.WindowSec, DoctorAgent: a.DoctorAgent, SelfRepairEnabled: a.SelfRepairEnabled, EscalateTo: a.EscalateTo, LastFallbackMS: a.LastFallbackMS, LastForcedMS: a.LastForcedMS, LastReason: a.LastReason, TaskType: a.TaskType, ForcedChain: a.ForcedChain, ForceGeneration: a.ForceGeneration})
	}
	return out
}

func convert[T, U any](rows []T, to func(T) U) []U {
	out := make([]U, 0, len(rows))
	for _, row := range rows {
		out = append(out, to(row))
	}
	return out
}

// Scan judges agents idle for idle_days and artifacts older than stale_days,
// both measured back from the daemon's clock.
func (s *Service) Scan(_ context.Context, in ScanRequest) (ScanOutput, error) {
	idleDays, staleDays := days(in.IdleDays), days(in.StaleDays)
	now := s.now()
	rep := s.scan(now.Add(-time.Duration(idleDays)*24*time.Hour).UnixMilli(), now.Add(-time.Duration(staleDays)*24*time.Hour).UnixMilli())
	out := ScanOutput{
		DeadAgents: convert(rep.DeadAgents, func(a runtime.ReaperAgent) DeadRow {
			return DeadRow{Slug: a.Slug, Name: a.Name, LastActiveMS: a.LastActiveMS}
		}),
		DegradedAgents: convert(rep.DegradedAgents, func(a runtime.DegradedAgent) DegradedRow {
			return DegradedRow{Slug: a.Slug, Name: a.Name, Failures: a.Failures, Window: a.Window, Threshold: a.Threshold, DoctorAgent: a.DoctorAgent, SelfRepairEnabled: a.SelfRepairEnabled, EscalateTo: a.EscalateTo, LastFailureMS: a.LastFailureMS, LastReason: a.LastReason}
		}),
		MisconfiguredAgents: convert(rep.MisconfiguredAgents, func(a runtime.MisconfiguredAgent) MisconfiguredRow {
			return MisconfiguredRow{Slug: a.Slug, Name: a.Name, Issues: a.Issues, DoctorAgent: a.DoctorAgent, SelfRepairEnabled: a.SelfRepairEnabled, EscalateTo: a.EscalateTo}
		}),
		RetryPressureAgents: convert(rep.RetryPressure, func(a runtime.RetryPressureAgent) RetryRow {
			return RetryRow{Slug: a.Slug, Name: a.Name, Count: a.Count, Threshold: a.Threshold, WindowSec: a.WindowSec, DoctorAgent: a.DoctorAgent, SelfRepairEnabled: a.SelfRepairEnabled, EscalateTo: a.EscalateTo, LastRetryMS: a.LastRetryMS, LastReason: a.LastReason, NextAttempt: a.NextAttempt, MaxAttempts: a.MaxAttempts}
		}),
		RoutingPressureAgents: convert(rep.RoutingPressure, func(a runtime.RoutingPressureAgent) RoutingRow {
			return RoutingRow{Slug: a.Slug, Name: a.Name, Count: a.Count, Threshold: a.Threshold, WindowSec: a.WindowSec, DoctorAgent: a.DoctorAgent, SelfRepairEnabled: a.SelfRepairEnabled, EscalateTo: a.EscalateTo, LastFallbackMS: a.LastFallbackMS, LastReason: a.LastReason, LastFailedModel: a.LastFailedModel, LastNextModel: a.LastNextModel, TaskType: a.TaskType}
		}),
		ForcedProbationAgents: forced(rep.RoutingForced),
		ForcedFailedAgents: forced(convert(rep.RoutingForcedFailed, func(a runtime.RoutingForcedFailedAgent) runtime.RoutingForcedProbationAgent {
			return runtime.RoutingForcedProbationAgent(a)
		})),
		ForcedExhaustedAgents: forced(convert(rep.RoutingForcedExhausted, func(a runtime.RoutingForcedExhaustedAgent) runtime.RoutingForcedProbationAgent {
			return runtime.RoutingForcedProbationAgent(a)
		})),
		RoutingUnstableAgents: convert(rep.RoutingUnstable, func(a runtime.RoutingUnstableAgent) UnstableRow {
			return UnstableRow{Slug: a.Slug, Name: a.Name, Count: a.Count, Threshold: a.Threshold, WindowSec: a.WindowSec, DoctorAgent: a.DoctorAgent, SelfRepairEnabled: a.SelfRepairEnabled, EscalateTo: a.EscalateTo, LastRollbackMS: a.LastRollbackMS, TaskType: a.TaskType, CurrentChain: a.CurrentChain, PreviousChain: a.PreviousChain, LastReason: a.LastReason}
		}),
		StaleArtifacts: rep.StaleArtifacts,
		StaleBytes:     rep.StaleBytes,
		IdleDays:       idleDays,
		StaleDays:      staleDays,
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
