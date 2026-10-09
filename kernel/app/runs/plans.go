// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/platform/journalview"
)

// Plans folds one kernel's plan executions: a plan.started and a terminal
// plan.completed or plan.failed under one plan correlation.
type Plans struct{ journal journalview.Reader }

func NewPlans(journal journalview.Reader) *Plans { return &Plans{journal: journal} }

type PlansRequest struct {
	Limit  json.RawMessage `json:"limit,omitempty"`
	Cursor json.RawMessage `json:"cursor,omitempty"`
	Status json.RawMessage `json:"status,omitempty"`
}

// PlanStatsRequest takes no arguments.
type PlanStatsRequest struct{}

type PlanRow struct {
	CorrelationID string `json:"correlation_id"`
	PlanName      string `json:"plan_name"`
	NodeCount     int64  `json:"node_count"`
	Status        string `json:"status"`
	StartedUnixMS int64  `json:"started_unix_ms"`
	DurationMS    int64  `json:"duration_ms"`
}

type PlansOutput struct {
	Plans      []PlanRow `json:"plans"`
	Count      int       `json:"count"`
	NextCursor string    `json:"next_cursor"`
}

type PlanStatsOutput struct {
	Total       int          `json:"total"`
	Completed   int          `json:"completed"`
	Failed      int          `json:"failed"`
	Running     int          `json:"running"`
	Terminal    int          `json:"terminal"`
	SuccessRate float64      `json:"success_rate"`
	DurationMS  Distribution `json:"duration_ms"`
}

type plan struct {
	corr, name         string
	nodeCount          int64
	startedMS, endedMS int64
	startSeq           int64
	status             string
}

// lifecycle reads plan_name and node_count; a malformed payload is zero.
func lifecycle(payload json.RawMessage) (string, int64) {
	var p struct {
		PlanName  string `json:"plan_name"`
		NodeCount int64  `json:"node_count"`
	}
	if len(payload) == 0 || json.Unmarshal(payload, &p) != nil {
		return "", 0
	}
	return p.PlanName, p.NodeCount
}

// fold joins each plan correlation's start and terminal events. A plan with no
// terminal event is running; a terminal event names a plan whose start did not.
func (s *Plans) fold() (map[string]*plan, error) {
	plans := map[string]*plan{}
	get := func(corr string) *plan {
		p := plans[corr]
		if p == nil {
			p = &plan{corr: corr, status: "running"}
			plans[corr] = p
		}
		return p
	}
	err := s.journal.Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindPlanStarted:
			p := get(e.CorrelationID)
			p.name, p.nodeCount = lifecycle(e.Payload)
			p.startedMS, p.startSeq = e.TSUnixMS, e.Seq
		case event.KindPlanCompleted, event.KindPlanFailed:
			p := get(e.CorrelationID)
			p.status, p.endedMS = map[event.Kind]string{event.KindPlanCompleted: "completed", event.KindPlanFailed: "failed"}[e.Kind], e.TSUnixMS
			if p.name == "" {
				p.name, _ = lifecycle(e.Payload)
			}
		}
		return nil
	})
	return plans, err
}

func (p *plan) duration() (int64, bool) {
	if p.endedMS > 0 && p.startedMS > 0 && p.endedMS >= p.startedMS {
		return p.endedMS - p.startedMS, true
	}
	return 0, false
}

// List pages the plan executions newest start first. A plan whose start was not
// journaled sorts as the oldest, and ties among those break by correlation.
func (s *Plans) List(_ context.Context, in PlansRequest) (PlansOutput, error) {
	limit := defaultListLimit
	if v, ok := rawValue(in.Limit).(float64); ok {
		limit = int(v)
	}
	limit = min(max(limit, 1), maxListLimit)
	cursorMS, cursorSeq, cursorOK := journal.DecodeCursor(rawValue(in.Cursor))
	status, err := optionalString(in.Status, "status")
	if err != nil {
		return PlansOutput{}, err
	}
	plans, err := s.fold()
	if err != nil {
		return PlansOutput{}, err
	}
	rows := make([]*plan, 0, len(plans))
	for _, p := range plans {
		if status != "" && p.status != status || cursorOK && !journal.KeepBeforeCursor(p.startedMS, p.startSeq, cursorMS, cursorSeq) {
			continue
		}
		rows = append(rows, p)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].startedMS != rows[j].startedMS {
			return rows[i].startedMS > rows[j].startedMS
		}
		if rows[i].startSeq != rows[j].startSeq {
			return rows[i].startSeq > rows[j].startSeq
		}
		return rows[i].corr < rows[j].corr
	})
	rows = rows[:min(len(rows), limit)]
	out := PlansOutput{Plans: make([]PlanRow, 0, len(rows)), Count: len(rows)}
	for _, p := range rows {
		d, _ := p.duration()
		out.Plans = append(out.Plans, PlanRow{CorrelationID: p.corr, PlanName: p.name, NodeCount: p.nodeCount, Status: p.status, StartedUnixMS: p.startedMS, DurationMS: d})
	}
	if n := len(rows); n > 0 {
		out.NextCursor = journal.NextCursor(rows[n-1].startedMS, rows[n-1].startSeq, n, limit)
	}
	return out, nil
}

// Stats counts the plans by status, the success rate over terminal plans and the
// duration distribution of terminal plans with a journaled start.
func (s *Plans) Stats(context.Context, PlanStatsRequest) (PlanStatsOutput, error) {
	plans, err := s.fold()
	if err != nil {
		return PlanStatsOutput{}, err
	}
	var out PlanStatsOutput
	durations := make([]int64, 0, len(plans))
	for _, p := range plans {
		out.Total++
		switch p.status {
		case "completed":
			out.Completed++
		case "failed":
			out.Failed++
		default:
			out.Running++
		}
		if d, ok := p.duration(); ok {
			durations = append(durations, d)
		}
	}
	out.Terminal = out.Completed + out.Failed
	if out.Terminal > 0 {
		out.SuccessRate = float64(out.Completed) / float64(out.Terminal)
	}
	out.DurationMS = distribution(durations)
	return out, nil
}

// PlanOperations declares the two unaudited plan reads. Each routes to the
// caller's tenant kernel, so a tenant sees only its own plans.
func PlanOperations(provider func(context.Context) *Plans) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("plans provider required")
	}
	var ops []app.Operation
	if err := bind(&ops, opapi.Spec{Name: "plan_history", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"limit":{},"cursor":{},"status":{}}}`), HTTP: opapi.HTTP{Method: "GET", Path: "/api/plan_history"}}, func(ctx context.Context, in PlansRequest) (PlansOutput, error) {
		return provider(ctx).List(ctx, in)
	}); err != nil {
		return nil, err
	}
	if err := bind(&ops, opapi.Spec{Name: "plan_stats", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true}`)}, func(ctx context.Context, in PlanStatsRequest) (PlanStatsOutput, error) {
		return provider(ctx).Stats(ctx, in)
	}); err != nil {
		return nil, err
	}
	return ops, nil
}
