// SPDX-License-Identifier: MIT

package controlplane

// Plan-execution history (M83) — the plan analogue of `agt runs list`. A plan
// run (the SPEC-02 scheduler executing gate/loop nodes) journals a plan.started
// and a terminal plan.completed / plan.failed, all under the same "plan-…"
// correlation. Those runs aren't task runs, so collectRuns / `agt runs list`
// never see them — an operator had no way to discover "what plans have run?".
// This folds the plan lifecycle events so they're listable, newest-first, with
// each plan's outcome and duration. Drill into one with `agt runs show <corr>`
// (M82).

import (
	"encoding/json"
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/event"
)

func (s *Server) handlePlanHistory(conn net.Conn, req Request) {
	limit := defaultRunsLimit
	if raw, ok := req.Args["limit"]; ok {
		switch v := raw.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		case int64:
			limit = int(v)
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > maxRunsLimit {
		limit = maxRunsLimit
	}
	statusFilter, _ := req.Args["status"].(string) // completed|failed|running

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	type planRun struct {
		corr      string
		name      string
		nodeCount int64
		startedMS int64
		startSeq  int64
		endedMS   int64
		status    string // running until a terminal event is seen
	}
	plans := map[string]*planRun{}
	get := func(corr string) *planRun {
		p := plans[corr]
		if p == nil {
			p = &planRun{corr: corr, status: "running"}
			plans[corr] = p
		}
		return p
	}
	if err := k.Journal().Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindPlanStarted:
			name, nodeCount := decodePlanLifecycle(e.Payload)
			p := get(e.CorrelationID)
			p.name = name
			p.nodeCount = nodeCount
			p.startedMS = e.TSUnixMS
			p.startSeq = e.Seq
		case event.KindPlanCompleted:
			p := get(e.CorrelationID)
			p.status = "completed"
			p.endedMS = e.TSUnixMS
			if p.name == "" {
				p.name, _ = decodePlanLifecycle(e.Payload)
			}
		case event.KindPlanFailed:
			p := get(e.CorrelationID)
			p.status = "failed"
			p.endedMS = e.TSUnixMS
			if p.name == "" {
				p.name, _ = decodePlanLifecycle(e.Payload)
			}
		}
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	rows := make([]*planRun, 0, len(plans))
	for _, p := range plans {
		if statusFilter != "" && p.status != statusFilter {
			continue
		}
		rows = append(rows, p)
	}
	// Newest plan first; seq breaks a same-millisecond tie.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].startedMS != rows[j].startedMS {
			return rows[i].startedMS > rows[j].startedMS
		}
		return rows[i].startSeq > rows[j].startSeq
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}

	out := make([]map[string]any, 0, len(rows))
	for _, p := range rows {
		var duration int64
		if p.endedMS > 0 && p.startedMS > 0 && p.endedMS >= p.startedMS {
			duration = p.endedMS - p.startedMS
		}
		out = append(out, map[string]any{
			"correlation_id":  p.corr,
			"plan_name":       p.name,
			"node_count":      p.nodeCount,
			"status":          p.status,
			"started_unix_ms": p.startedMS,
			"duration_ms":     duration,
		})
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"plans": out, "count": len(out)},
	})
}

// handlePlanStats aggregates plan executions (M84) — the plan analogue of
// handleRunsStats. Folds the plan lifecycle into total / completed / failed /
// running counts, a success rate, and a duration distribution over terminal
// plans. Tenant-routed (primary-only, like CmdPlan).
func (s *Server) handlePlanStats(conn net.Conn, req Request) {
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	type planRun struct {
		startedMS, endedMS int64
		status             string
	}
	plans := map[string]*planRun{}
	get := func(corr string) *planRun {
		p := plans[corr]
		if p == nil {
			p = &planRun{status: "running"}
			plans[corr] = p
		}
		return p
	}
	if err := k.Journal().Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindPlanStarted:
			get(e.CorrelationID).startedMS = e.TSUnixMS
		case event.KindPlanCompleted:
			p := get(e.CorrelationID)
			p.status = "completed"
			p.endedMS = e.TSUnixMS
		case event.KindPlanFailed:
			p := get(e.CorrelationID)
			p.status = "failed"
			p.endedMS = e.TSUnixMS
		}
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	var total, completed, failed, running int
	durations := make([]int64, 0, len(plans))
	for _, p := range plans {
		total++
		switch p.status {
		case "completed":
			completed++
		case "failed":
			failed++
		default:
			running++
		}
		if p.endedMS > 0 && p.startedMS > 0 && p.endedMS >= p.startedMS {
			durations = append(durations, p.endedMS-p.startedMS)
		}
	}
	terminal := completed + failed
	successRate := 0.0
	if terminal > 0 {
		successRate = float64(completed) / float64(terminal)
	}
	dstats := durationStats(durations)

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"total":        total,
			"completed":    completed,
			"failed":       failed,
			"running":      running,
			"terminal":     terminal,
			"success_rate": successRate,
			"duration_ms": map[string]any{
				"count": len(durations),
				"avg":   dstats.avg,
				"min":   dstats.min,
				"max":   dstats.max,
				"p50":   dstats.p50,
				"p95":   dstats.p95,
			},
		},
	})
}

// decodePlanLifecycle pulls plan_name + node_count out of a plan.started /
// plan.completed payload (M83). Returns zero values on parse failure.
func decodePlanLifecycle(payload json.RawMessage) (name string, nodeCount int64) {
	if len(payload) == 0 {
		return "", 0
	}
	var p struct {
		PlanName  string `json:"plan_name"`
		NodeCount int64  `json:"node_count"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", 0
	}
	return p.PlanName, p.NodeCount
}
