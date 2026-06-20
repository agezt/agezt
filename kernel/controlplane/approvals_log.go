// SPDX-License-Identifier: MIT

package controlplane

// Approval-decision audit log (M87) — a read-only timeline of the journal's
// approval.requested joined with the terminal approval.granted / approval.denied
// / approval.timeout events. `agt approvals` shows what's PENDING; this shows the
// HISTORY of HITL gating: what was asked, how it resolved, and who decided. The
// human-in-the-loop analogue of `agt edict log` (which audits the automatic
// policy gating) — together they cover both halves of "was this allowed?".

import (
	"encoding/json"
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/event"
)

// handleApprovalsStats aggregates HITL approvals (M88) — total / granted /
// denied / timeout, a grant rate over resolved requests, and a denied-by-
// capability breakdown. The human analogue of handleEdictStats. since_ms windows
// by request time. Tenant-routed.
func (s *Server) handleApprovalsStats(conn net.Conn, req Request) {
	cutoff := sinceCutoff(req.Args["since_ms"])
	var sinceMS int64
	switch v := req.Args["since_ms"].(type) {
	case float64:
		sinceMS = int64(v)
	case int64:
		sinceMS = v
	case int:
		sinceMS = int64(v)
	}

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	// Status + capability per approval_id, folded across request + resolution.
	type ap struct {
		ts         int64
		capability string
		status     string
	}
	byID := map[string]*ap{}
	get := func(id string) *ap {
		a := byID[id]
		if a == nil {
			a = &ap{status: "pending"}
			byID[id] = a
		}
		return a
	}
	if err := k.Journal().Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindApprovalRequested:
			var p struct {
				ApprovalID string `json:"approval_id"`
				Capability string `json:"capability"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			if p.ApprovalID == "" {
				return nil
			}
			a := get(p.ApprovalID)
			a.ts = e.TSUnixMS
			a.capability = p.Capability
		case event.KindApprovalGranted, event.KindApprovalDenied, event.KindApprovalTimeout:
			var p struct {
				ApprovalID string `json:"approval_id"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			if p.ApprovalID == "" {
				return nil
			}
			a := get(p.ApprovalID)
			switch e.Kind {
			case event.KindApprovalGranted:
				a.status = "granted"
			case event.KindApprovalDenied:
				a.status = "denied"
			case event.KindApprovalTimeout:
				a.status = "timeout"
			}
		}
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	var total, granted, denied, timeout, pending int
	deniedByCap := map[string]int{}
	for _, a := range byID {
		if cutoff > 0 && a.ts < cutoff {
			continue
		}
		total++
		switch a.status {
		case "granted":
			granted++
		case "denied":
			denied++
		case "timeout":
			timeout++
		default:
			pending++
		}
		if a.status == "denied" || a.status == "timeout" {
			capName := a.capability
			if capName == "" {
				capName = "unknown"
			}
			deniedByCap[capName]++
		}
	}
	resolved := granted + denied + timeout
	grantRate := 0.0
	if resolved > 0 {
		grantRate = float64(granted) / float64(resolved)
	}

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"total":                total,
			"granted":              granted,
			"denied":               denied,
			"timeout":              timeout,
			"pending":              pending,
			"resolved":             resolved,
			"grant_rate":           grantRate,
			"denied_by_capability": deniedByCap,
			"window_ms":            sinceMS,
		},
	})
}

func (s *Server) handleApprovalsLog(conn net.Conn, req Request) {
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
	deniedOnly, _ := req.Args["denied"].(bool)
	cutoff := sinceCutoff(req.Args["since_ms"]) // M65 helper

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	type approval struct {
		ts, seq                      int64
		id, capability, tool, reason string
		actor, correlationID         string
		status                       string // pending | granted | denied | timeout
		resolvedBy                   string
	}
	// One row per request (always present); the terminal event updates its
	// status. Keyed by approval_id so the request and its resolution join.
	byID := map[string]*approval{}
	order := make([]*approval, 0)
	get := func(id string, ts, seq int64) *approval {
		a := byID[id]
		if a == nil {
			a = &approval{id: id, ts: ts, seq: seq, status: "pending"}
			byID[id] = a
			order = append(order, a)
		}
		return a
	}
	if err := k.Journal().Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindApprovalRequested:
			var p struct {
				ApprovalID string `json:"approval_id"`
				Capability string `json:"capability"`
				ToolName   string `json:"tool_name"`
				Reason     string `json:"reason"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			if p.ApprovalID == "" {
				return nil
			}
			a := get(p.ApprovalID, e.TSUnixMS, e.Seq)
			a.ts, a.seq = e.TSUnixMS, e.Seq // requested time anchors the row
			a.capability, a.tool, a.reason = p.Capability, p.ToolName, p.Reason
			a.actor, a.correlationID = e.Actor, e.CorrelationID
		case event.KindApprovalGranted, event.KindApprovalDenied, event.KindApprovalTimeout:
			var p struct {
				ApprovalID string `json:"approval_id"`
				ResolvedBy string `json:"resolved_by"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			if p.ApprovalID == "" {
				return nil
			}
			a := get(p.ApprovalID, e.TSUnixMS, e.Seq)
			if a.actor == "" {
				a.actor = e.Actor
			}
			if a.correlationID == "" {
				a.correlationID = e.CorrelationID
			}
			switch e.Kind {
			case event.KindApprovalGranted:
				a.status = "granted"
			case event.KindApprovalDenied:
				a.status = "denied"
			case event.KindApprovalTimeout:
				a.status = "timeout"
			}
			a.resolvedBy = p.ResolvedBy
		}
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	rows := make([]*approval, 0, len(order))
	for _, a := range order {
		if cutoff > 0 && a.ts < cutoff {
			continue
		}
		if deniedOnly && a.status != "denied" && a.status != "timeout" {
			continue
		}
		rows = append(rows, a)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ts != rows[j].ts {
			return rows[i].ts > rows[j].ts
		}
		return rows[i].seq > rows[j].seq
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}

	out := make([]map[string]any, 0, len(rows))
	for _, a := range rows {
		out = append(out, map[string]any{
			"ts_unix_ms":     a.ts,
			"approval_id":    a.id,
			"capability":     a.capability,
			"tool":           a.tool,
			"reason":         a.reason,
			"actor":          a.actor,
			"correlation_id": a.correlationID,
			"status":         a.status,
			"resolved_by":    a.resolvedBy,
		})
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"approvals": out, "count": len(out)},
	})
}
