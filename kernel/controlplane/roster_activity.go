// SPDX-License-Identifier: MIT

package controlplane

// Agent activity + repair-status handlers (M833/M846): the read-only
// operator timeline of journal events for one agent, and the auto-repair
// state snapshot. Carved out of roster.go during the Day 24 god file
// split #5 so the main file can focus on mutation handlers (add/edit/
// remove/wake/resolve/escalation).

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
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

func agentRepairContractView(p roster.Profile, cooldown time.Duration) map[string]any {
	retryAttempts := 1
	retryBackoff := "none"
	retryOn := []string{"error", "timeout"}
	if p.RetryPolicy != nil {
		if p.RetryPolicy.MaxAttempts > 0 {
			retryAttempts = p.RetryPolicy.MaxAttempts
		}
		if strings.TrimSpace(p.RetryPolicy.Backoff) != "" {
			retryBackoff = strings.TrimSpace(p.RetryPolicy.Backoff)
		}
		if len(p.RetryPolicy.RetryOn) > 0 {
			retryOn = append([]string(nil), p.RetryPolicy.RetryOn...)
		}
	}
	selfRepairEnabled := p.SelfRepairPolicy != nil && p.SelfRepairPolicy.Enabled
	selfRepairMax := 0
	escalateTo := ""
	if p.SelfRepairPolicy != nil {
		selfRepairMax = p.SelfRepairPolicy.MaxAttempts
		escalateTo = strings.TrimSpace(p.SelfRepairPolicy.EscalateTo)
	}
	doctor := ""
	failureThreshold := 0
	if p.HealthPolicy != nil {
		doctor = strings.TrimSpace(p.HealthPolicy.DoctorAgent)
		failureThreshold = p.HealthPolicy.FailureThreshold
	}
	return map[string]any{
		"retry_attempts":       retryAttempts,
		"retry_backoff":        retryBackoff,
		"retry_on":             retryOn,
		"doctor_agent":         doctor,
		"failure_threshold":    failureThreshold,
		"self_repair_enabled":  selfRepairEnabled,
		"self_repair_attempts": selfRepairMax,
		"escalate_to":          escalateTo,
		"cooldown_sec":         int(cooldown / time.Second),
		"authority_boundary":   "agent identity owns retry, doctor, self-repair and escalation; schedules/workflows only wake this contract",
	}
}

func agentRepairNextActionView(p roster.Profile, rows, inflight []agentRepairRow, nowMS int64) map[string]any {
	action := "manual_repair"
	label := "manual repair"
	detail := "no autonomous repair is currently queued"
	tone := "muted"
	if p.Retired {
		return map[string]any{"action": "revive_required", "label": "revive required", "detail": "graveyard agent cannot repair until revived", "tone": "muted"}
	}
	if !p.Enabled {
		return map[string]any{"action": "resume_required", "label": "resume required", "detail": "paused agent cannot repair until resumed", "tone": "warn"}
	}
	if len(inflight) > 0 {
		row := inflight[0]
		return map[string]any{
			"action":         "wait_inflight",
			"label":          "repair in flight",
			"detail":         repairDecisionDetail(row, "doctor/self-repair run is already queued"),
			"tone":           "accent",
			"correlation_id": row.CorrelationID,
			"fingerprint":    row.Fingerprint,
			"phase":          row.Phase,
		}
	}
	var latest agentRepairRow
	if len(rows) > 0 {
		latest = rows[0]
		if latest.NextEligibleMS > nowMS {
			return map[string]any{
				"action":           "cooldown",
				"label":            "cooldown active",
				"detail":           repairDecisionDetail(latest, "wait before another autonomous repair attempt"),
				"tone":             "warn",
				"next_eligible_ms": latest.NextEligibleMS,
				"phase":            latest.Phase,
				"fingerprint":      latest.Fingerprint,
			}
		}
		switch strings.TrimSpace(latest.Phase) {
		case "attempts_exhausted", "resolution_failed", "routing_rollback_failed", "failed":
			target := firstNonEmpty(strings.TrimSpace(latest.DelegateTo), repairEscalationOwner(p))
			if target != "" {
				return map[string]any{
					"action":      "escalate_owner",
					"label":       "escalate owner",
					"detail":      repairDecisionDetail(latest, "self-repair failed; owner should take over"),
					"tone":        "bad",
					"delegate_to": target,
					"phase":       latest.Phase,
				}
			}
			return map[string]any{
				"action": "operator_resolution",
				"label":  "operator resolution",
				"detail": repairDecisionDetail(latest, "repair failed and no owner escalation target is configured"),
				"tone":   "bad",
				"phase":  latest.Phase,
			}
		}
	}
	if p.SelfRepairPolicy != nil && p.SelfRepairPolicy.Enabled {
		action = "run_self_repair"
		label = "self-repair eligible"
		detail = "next failure can trigger autonomous self-repair"
		tone = "good"
	} else if p.HealthPolicy != nil && strings.TrimSpace(p.HealthPolicy.DoctorAgent) != "" {
		action = "doctor_monitor"
		label = "doctor monitoring"
		detail = "doctor can queue repair after health threshold"
		tone = "good"
	}
	if latest.Phase != "" {
		detail = repairDecisionDetail(latest, detail)
	}
	return map[string]any{"action": action, "label": label, "detail": detail, "tone": tone}
}

func repairEscalationOwner(p roster.Profile) string {
	if p.SelfRepairPolicy != nil && strings.TrimSpace(p.SelfRepairPolicy.EscalateTo) != "" {
		return strings.TrimSpace(p.SelfRepairPolicy.EscalateTo)
	}
	return firstNonEmpty(strings.TrimSpace(p.ParentAgent), strings.TrimSpace(p.OwnerAgent))
}

func repairDecisionDetail(row agentRepairRow, fallback string) string {
	parts := []string{fallback}
	if row.Mode != "" {
		parts = append(parts, "mode "+row.Mode)
	}
	if row.Phase != "" {
		parts = append(parts, "phase "+row.Phase)
	}
	if row.Fingerprint != "" {
		parts = append(parts, "fingerprint "+row.Fingerprint)
	}
	if row.Reason != "" {
		parts = append(parts, row.Reason)
	} else if row.Error != "" {
		parts = append(parts, row.Error)
	}
	if row.SelfRepairAttempt > 0 && row.SelfRepairMaxAttempts > 0 {
		parts = append(parts, fmt.Sprintf("attempt %d/%d", row.SelfRepairAttempt, row.SelfRepairMaxAttempts))
	}
	return strings.Join(parts, " · ")
}

