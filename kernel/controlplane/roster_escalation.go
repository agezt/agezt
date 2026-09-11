// SPDX-License-Identifier: MIT

package controlplane

// Agent escalation handler (M846): the open-doctor-responsibilities view
// plus the operator-incident lineage bookkeeping that decides which
// escalation events to surface. Carved out of roster.go during the Day 24
// god file split #9 so the main file can shrink to ~200 lines of register
// + dispatch only.

import (
	"context"
	"net"
	"strconv"
	"strings"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
)

func (s *Server) handleAgentEscalations(conn net.Conn, req Request) {
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
	st, err := s.boardReader()
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	// Cursor pagination (M-pending follow-up): `cursor` is the opaque
	// "<ts_unix_ms>:<message_id>" boundary of the previous page; server skips
	// entries strictly newer-or-equal. ts can collide across messages, so the
	// message_id is the tie-break.
	var cursorTS int64
	var cursorID string
	cursorOK := false
	if raw, _, cerr := argString(req.Args, "cursor"); cerr != nil {
		s.fail(conn, req, cerr)
		return
	} else if raw != "" {
		tsStr, id, _ := strings.Cut(raw, ":")
		if ts, perr := strconv.ParseInt(tsStr, 10, 64); perr == nil {
			cursorTS, cursorID, cursorOK = ts, id, true
		}
	}
	if !cursorOK {
		cursorTS, cursorID = 0, ""
	}
	rows, nextCursor := s.agentEscalationRows(st, p.Slug, limit, cursorTS, cursorID)
	openCount := 0
	for _, row := range rows {
		if row.Status == "open" {
			openCount++
		}
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"message_id":          row.MessageID,
			"from":                row.From,
			"to":                  row.To,
			"text":                row.Text,
			"ts_unix_ms":          row.TSUnixMS,
			"status":              row.Status,
			"reply_count":         row.ReplyCount,
			"acked":               row.Acked,
			"source_agent":        row.SourceAgent,
			"mode":                row.Mode,
			"wake_phase":          row.WakePhase,
			"wake_reason":         row.WakeReason,
			"wake_error":          row.WakeError,
			"wake_correlation_id": row.WakeCorrelationID,
			"fingerprint":         row.Fingerprint,
			"resolution":          row.Resolution,
			"resolution_summary":  row.ResolutionSummary,
			"delegate_to":         row.DelegateTo,
			"origin_kind":         row.OriginKind,
			"origin_agent":        row.OriginAgent,
			"root_agent":          row.RootAgent,
			"chain_depth":         row.ChainDepth,
			"incident_id":         row.IncidentID,
			"root_incident_id":    row.RootIncidentID,
			"parent_incident_id":  row.ParentIncidentID,
		})
	}
	result := map[string]any{
		"slug":        p.Slug,
		"escalations": out,
		"count":       len(out),
		"open_count":  openCount,
	}
	if nextCursor != "" {
		result["next_cursor"] = nextCursor
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

type operatorWakeLineage struct {
	incidentID       string
	rootIncidentID   string
	parentIncidentID string
}

func operatorIncidentLineage(args map[string]any) operatorWakeLineage {
	return operatorWakeLineage{
		incidentID:       strings.TrimSpace(stringArg(args, "incident_id")),
		rootIncidentID:   strings.TrimSpace(stringArg(args, "root_incident_id")),
		parentIncidentID: strings.TrimSpace(stringArg(args, "parent_incident_id")),
	}
}

// agentAutonomyRunbookPayload delegates to the canonical roster builder so manual
// operator wakes share the exact runbook shape as schedule/standing/delegated wakes.
func agentAutonomyRunbookPayload(p roster.Profile) map[string]any {
	return roster.AutonomyRunbook(p)
}

func publishOperatorAction(k *runtime.Kernel, subject, corr string, payload map[string]any) {
	if k == nil || k.Bus() == nil {
		return
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       subject,
		Kind:          event.KindInfo,
		Actor:         "controlplane",
		CorrelationID: corr,
		Payload:       payload,
	})
}

func buildOperatorWakeIntent(explicit, slug, reason string, args map[string]any) string {
	if text := strings.TrimSpace(explicit); text != "" {
		return text
	}
	var b strings.Builder
	b.WriteString("Manual wake-up.\n")
	b.WriteString("You are agent ")
	b.WriteString(slug)
	b.WriteString(". You were explicitly woken by the operator/control plane.\n")
	if reason = strings.TrimSpace(reason); reason != "" {
		b.WriteString("Reason: ")
		b.WriteString(reason)
		b.WriteString("\n")
	}
	if root := strings.TrimSpace(stringArg(args, "root_incident_id")); root != "" {
		b.WriteString("Incident root: ")
		b.WriteString(root)
		b.WriteString("\n")
	}
	if incident := strings.TrimSpace(stringArg(args, "incident_id")); incident != "" {
		b.WriteString("Incident hop: ")
		b.WriteString(incident)
		b.WriteString("\n")
	}
	b.WriteString("Inspect your durable instructions, memory, mailbox, tasklist, and current health context. Do the next concrete recovery step and then stop.")
	return b.String()
}

func (s *Server) runAgentWake(corr string, p roster.Profile, intent, reason string, lineage operatorWakeLineage) {
	runbook := agentAutonomyRunbookPayload(p)
	ctx := runtime.WithAgentProfile(context.Background(), p)
	ctx = runtime.WithWakeContext(ctx, runtime.WakeContext{
		Source: "operator",
		Reason: reason,
	})
	if p.MaxCostMc > 0 {
		ctx = runtime.WithMaxCost(ctx, p.MaxCostMc)
	}
	var (
		answer string
		err    error
	)
	if p.RetryPolicy != nil && p.RetryPolicy.MaxAttempts > 1 {
		answer, err = s.k.RunWithRetry(ctx, corr, intent, *p.RetryPolicy)
	} else {
		answer, err = s.k.RunWith(ctx, corr, intent)
	}
	if err != nil {
		publishOperatorAction(s.k, "agent.wake", corr, map[string]any{
			"phase":              "failed",
			"agent":              p.Slug,
			"reason":             reason,
			"error":              err.Error(),
			"autonomy_runbook":   runbook,
			"incident_id":        lineage.incidentID,
			"root_incident_id":   lineage.rootIncidentID,
			"parent_incident_id": lineage.parentIncidentID,
		})
		return
	}
	publishOperatorAction(s.k, "agent.wake", corr, map[string]any{
		"phase":              "completed",
		"agent":              p.Slug,
		"reason":             reason,
		"answer":             truncate(answer, 300),
		"autonomy_runbook":   runbook,
		"incident_id":        lineage.incidentID,
		"root_incident_id":   lineage.rootIncidentID,
		"parent_incident_id": lineage.parentIncidentID,
	})
}

// agentActivitySummary decides whether one event belongs in an agent's timeline
// and renders a one-line summary. Attribution is by the slug fields the events
// already carry, plus the agent's own run correlations for run-scoped events.
