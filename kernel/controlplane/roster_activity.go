// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"net"
	"sort"
	"strconv"
	"time"

	"github.com/agezt/agezt/kernel/event"
)


func (s *Server) handleAgentActivity(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	slug := p.Slug
	limit, err := argLimit(req.Args, 50, 500)
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	// Pass 1: the correlation ids of runs this agent executed (task.received
	// carries the agent slug since M854). These also scope the council consults
	// and delegations that happened *during* the agent's runs.
	// Also collect activity events in the same pass to avoid O(2n) journal walks.
	runCorr := map[string]bool{}
	var items []map[string]any
	_ = s.k.Journal().Range(func(e *event.Event) error {
		// Build runCorr map
		if e.Kind == event.KindTaskReceived {
			var pl map[string]any
			if json.Unmarshal(e.Payload, &pl) == nil && plString(pl, "agent") == slug && e.CorrelationID != "" {
				runCorr[e.CorrelationID] = true
			}
		}
		// Check if this event is attributable to the agent
		var pl map[string]any
		_ = json.Unmarshal(e.Payload, &pl)
		summary, ok := agentActivitySummary(e, pl, slug, runCorr)
		if !ok {
			return nil
		}
		items = append(items, map[string]any{
			"seq":            e.Seq,
			"kind":           string(e.Kind),
			"ts_unix_ms":     e.TSUnixMS,
			"correlation_id": e.CorrelationID,
			"summary":        summary,
		})
		return nil
	})

	// Newest first, capped.
	sort.SliceStable(items, func(i, j int) bool {
		return items[i]["seq"].(int64) > items[j]["seq"].(int64)
	})
	total := len(items)

	// Cursor pagination (M-pending follow-up): the SPA's IncidentPage /
	// AgentPage views load this on every poll, and the journal can hold tens
	// of thousands of events. `cursor` is the opaque "<seq>" boundary of the
	// previous page; the server skips entries with seq >= cursorSeq (the list
	// is already sorted DESC, so strictly-older means strictly-smaller seq).
	cursorSeq, cursorOK := parseSeqCursor(stringArg(req.Args, "cursor"))
	if cursorOK {
		filtered := items[:0]
		for _, it := range items {
			if it["seq"].(int64) >= cursorSeq {
				continue
			}
			filtered = append(filtered, it)
		}
		items = filtered
	}
	var nextCursor string
	if limit > 0 && len(items) > limit {
		items = items[:limit]
		nextCursor = strconv.FormatInt(items[limit-1]["seq"].(int64), 10)
	}
	result := map[string]any{
		"slug": slug, "activity": items, "count": len(items), "total": total,
	}
	if nextCursor != "" {
		result["next_cursor"] = nextCursor
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

// handleAgentRepairStatus folds the journal into one agent's autonomous
// self-repair history: queued/completed/failed doctor.auto_repair events,
// newest first, plus the current inflight fingerprints and effective cooldown.
func (s *Server) handleAgentRepairStatus(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	limit, err := argLimit(req.Args, 20, 100)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	cooldown := agentAutoRepairCooldown()
	var rows []agentRepairRow
	latestByFingerprint := map[string]agentRepairRow{}
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "doctor.auto_repair" || e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil || plString(pl, "agent") != p.Slug {
			return nil
		}
		row := agentRepairRow{
			Seq:                            e.Seq,
			TSUnixMS:                       e.TSUnixMS,
			Agent:                          plString(pl, "agent"),
			CorrelationID:                  e.CorrelationID,
			Mode:                           plString(pl, "mode"),
			Phase:                          plString(pl, "phase"),
			Reason:                         plString(pl, "reason"),
			Fingerprint:                    plString(pl, "fingerprint"),
			SelfRepairAttempt:              plInt(pl, "self_repair_attempt"),
			SelfRepairMaxAttempts:          plInt(pl, "self_repair_max_attempts"),
			Issues:                         plStrings(pl, "issues"),
			Applied:                        plStrings(pl, "applied"),
			Answer:                         plString(pl, "answer"),
			Error:                          plString(pl, "error"),
			NextEligibleMS:                 e.TSUnixMS + cooldown.Milliseconds(),
			Resolution:                     plString(pl, "resolution"),
			ResolutionSummary:              plString(pl, "resolution_summary"),
			DelegateTo:                     plString(pl, "delegate_to"),
			DelegatedBy:                    plString(pl, "delegated_by"),
			RootAgent:                      plString(pl, "root_agent"),
			ChainDepth:                     intNumber(pl["chain_depth"]),
			IncidentID:                     plString(pl, "incident_id"),
			RootIncidentID:                 plString(pl, "root_incident_id"),
			ParentIncidentID:               plString(pl, "parent_incident_id"),
			RoutingTaskType:                plString(pl, "routing_task_type"),
			RoutingTaskModelChain:          plStrings(pl, "routing_task_model_chain"),
			PreviousRoutingTaskModelChain:  plStrings(pl, "previous_routing_task_model_chain"),
			RoutingForceGeneration:         intNumber(pl["routing_force_generation"]),
			PreviousRoutingForceGeneration: intNumber(pl["previous_routing_force_generation"]),
		}
		rows = append(rows, row)
		if row.Fingerprint != "" {
			latestByFingerprint[row.Fingerprint] = row
		}
		return nil
	})
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Seq > rows[j].Seq })
	total := len(rows)

	// Cursor pagination (M-pending follow-up): `cursor` is the opaque "<seq>"
	// boundary of the previous page; server skips rows with seq >= cursorSeq.
	// Note: the `latest`/`next_eligible_ms` fields below intentionally use the
	// FULL row list (rows[0]) — they reflect the agent's current state, not the
	// page being viewed, so they must not move with cursor pagination.
	cursorSeq, cursorOK := parseSeqCursor(stringArg(req.Args, "cursor"))
	history := rows
	if cursorOK {
		filtered := rows[:0]
		for _, r := range rows {
			if r.Seq >= cursorSeq {
				continue
			}
			filtered = append(filtered, r)
		}
		history = filtered
	}
	var nextCursor string
	if limit > 0 && len(history) > limit {
		history = history[:limit]
		nextCursor = strconv.FormatInt(history[limit-1].Seq, 10)
	}
	inflightRows := make([]agentRepairRow, 0, len(latestByFingerprint))
	for _, row := range latestByFingerprint {
		if row.Phase == "queued" || row.Phase == "routing_rollback_queued" {
			inflightRows = append(inflightRows, row)
		}
	}
	sort.SliceStable(inflightRows, func(i, j int) bool { return inflightRows[i].Seq > inflightRows[j].Seq })

	result := map[string]any{
		"slug":           p.Slug,
		"cooldown_sec":   int(cooldown / time.Second),
		"contract":       agentRepairContractView(p, cooldown),
		"history":        repairRowsView(history),
		"count":          len(history),
		"total":          total,
		"inflight":       repairRowsView(inflightRows),
		"inflight_count": len(inflightRows),
	}
	if nextCursor != "" {
		result["next_cursor"] = nextCursor
	}
	if len(rows) > 0 {
		result["latest"] = repairRowView(rows[0])
		result["next_eligible_ms"] = rows[0].NextEligibleMS
	}
	result["next_action"] = agentRepairNextActionView(p, rows, inflightRows, time.Now().UnixMilli())
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

