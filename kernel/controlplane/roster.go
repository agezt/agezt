// SPDX-License-Identifier: MIT

package controlplane

// Agent roster CRUD handlers (M783) — the management path behind `agt agent`.
// Lifecycle changes go through the kernel so every create/edit/pause/resume/
// remove is journaled (roster.*) and auditable via `agt why`. Profiles are
// addressed by ref = id OR slug everywhere, so operators can say
// `agt agent show researcher` without copying ULIDs.

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
)

type agentRepairRow struct {
	Seq                            int64
	TSUnixMS                       int64
	Agent                          string
	CorrelationID                  string
	Mode                           string
	Phase                          string
	Reason                         string
	Fingerprint                    string
	SelfRepairAttempt              int
	SelfRepairMaxAttempts          int
	Issues                         []string
	Applied                        []string
	Answer                         string
	Error                          string
	TargetAgent                    string
	TargetCorr                     string
	MailboxMessage                 string
	Resolution                     string
	ResolutionSummary              string
	DelegateTo                     string
	DelegatedBy                    string
	RootAgent                      string
	ChainDepth                     int
	IncidentID                     string
	RootIncidentID                 string
	ParentIncidentID               string
	NextEligibleMS                 int64
	RoutingTaskType                string
	RoutingTaskModelChain          []string
	PreviousRoutingTaskModelChain  []string
	RoutingForceGeneration         int
	PreviousRoutingForceGeneration int
}

type agentEscalationRow struct {
	MessageID         string
	From              string
	To                string
	Text              string
	TSUnixMS          int64
	Status            string
	ReplyCount        int
	Acked             bool
	SourceAgent       string
	Mode              string
	WakePhase         string
	WakeReason        string
	WakeError         string
	WakeCorrelationID string
	Fingerprint       string
	Resolution        string
	ResolutionSummary string
	DelegateTo        string
	OriginKind        string
	OriginAgent       string
	RootAgent         string
	ChainDepth        int
	IncidentID        string
	RootIncidentID    string
	ParentIncidentID  string
}

type agentRepairSummary struct {
	Latest        agentRepairRow
	HasLatest     bool
	InflightCount int
}

type agentRoutingPressure struct {
	Count      int
	LastReason string
	LastFailed string
	LastNext   string
	LastTSMS   int64
}

type agentRetryPressure struct {
	Count       int
	LastReason  string
	LastTSMS    int64
	NextAttempt int
	MaxAttempts int
}

type agentEscalationLoad struct {
	Open  int
	Acked int
}

type agentWakeStatus struct {
	ScheduleCount       int
	StandingCount       int
	EventSubjects       []string
	NextScheduledWakeMS int64
	NextScheduledLabel  string
}

type agentLiveStatus struct {
	ActiveRuns              int
	ActiveCorrelationID     string
	ActiveIntent            string
	ActiveStartedMS         int64
	ActiveModel             string
	ActiveSpentMc           int64
	ActivePhase             string
	ActiveLastEventMS       int64
	ActiveLastEventKind     string
	ActiveDetail            string
	ActiveTool              string
	ActiveIter              int
	ActiveWakeSource        string
	ActiveWakeReason        string
	ActiveScheduleID        string
	ActiveStandingID        string
	ActiveStandingName      string
	ActiveTriggerSubject    string
	ActiveParentCorrelation string
}

type agentLastActivity struct {
	TSUnixMS      int64
	Kind          string
	CorrelationID string
	Summary       string
}


// agentStatusAccums holds the per-agent state that the journal-derivable
// helpers accumulate. Roster-agentList page is the single consumer; collecting
// every accumulator in a SINGLE journal.Range pass turns what used to be
// O(journalSize) per helper (and 11× that across all helpers) into one
// O(journalSize) walk. Large, busy journals were reliably tripping the
// control-plane connection's 10-minute read deadline under the previous
// 11-Range design (each Range does a callback-driven O(n) walk over every
// durable event, with JSON-unmarshal + map lookups per event). The
// single-pass dispatch below is behavior-preserving: each accumulator's
// key-set and "latest wins" semantics match the original per-helper
// implementations (the retired per-helper methods have been deleted; this
// dispatch is now the only journal-derived roster-status path).
func (s *Server) handleAgentImpact(conn net.Conn, req Request) {
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
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: s.agentImpactResult(p)})
}

// handleAgentTombstone returns a read-only death certificate for an agent: its
// identity, lifecycle/retirement record, and durable resource footprint. Portable
// archival/audit artifact — it removes and mutates nothing (NEXT.md #7).
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
func agentAutoRepairCooldown() time.Duration {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "AUTO_REPAIR_COOLDOWN"))
	if raw == "" {
		return 30 * time.Minute
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 30 * time.Minute
	}
	return d
}

func (s *Server) agentRepairSummaries() map[string]agentRepairSummary {
	cooldown := agentAutoRepairCooldown()
	latestBySlug := map[string]agentRepairRow{}
	latestBySlugFingerprint := map[string]map[string]agentRepairRow{}
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "doctor.auto_repair" || e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		slug := plString(pl, "agent")
		if strings.TrimSpace(slug) == "" {
			return nil
		}
		row := agentRepairRow{
			Seq:                            e.Seq,
			TSUnixMS:                       e.TSUnixMS,
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
			TargetAgent:                    plString(pl, "target_agent"),
			TargetCorr:                     plString(pl, "target_correlation"),
			MailboxMessage:                 plString(pl, "mailbox_message_id"),
			Resolution:                     plString(pl, "resolution"),
			ResolutionSummary:              plString(pl, "resolution_summary"),
			DelegateTo:                     plString(pl, "delegate_to"),
			DelegatedBy:                    plString(pl, "delegated_by"),
			RootAgent:                      plString(pl, "root_agent"),
			ChainDepth:                     intNumber(pl["chain_depth"]),
			IncidentID:                     plString(pl, "incident_id"),
			RootIncidentID:                 plString(pl, "root_incident_id"),
			ParentIncidentID:               plString(pl, "parent_incident_id"),
			NextEligibleMS:                 e.TSUnixMS + cooldown.Milliseconds(),
			RoutingTaskType:                plString(pl, "routing_task_type"),
			RoutingTaskModelChain:          plStrings(pl, "routing_task_model_chain"),
			PreviousRoutingTaskModelChain:  plStrings(pl, "previous_routing_task_model_chain"),
			RoutingForceGeneration:         intNumber(pl["routing_force_generation"]),
			PreviousRoutingForceGeneration: intNumber(pl["previous_routing_force_generation"]),
		}
		if cur, ok := latestBySlug[slug]; !ok || row.Seq > cur.Seq {
			latestBySlug[slug] = row
		}
		if row.Fingerprint != "" {
			if latestBySlugFingerprint[slug] == nil {
				latestBySlugFingerprint[slug] = map[string]agentRepairRow{}
			}
			if cur, ok := latestBySlugFingerprint[slug][row.Fingerprint]; !ok || row.Seq > cur.Seq {
				latestBySlugFingerprint[slug][row.Fingerprint] = row
			}
		}
		return nil
	})
	out := map[string]agentRepairSummary{}
	for slug, latest := range latestBySlug {
		sum := agentRepairSummary{Latest: latest, HasLatest: true}
		for _, row := range latestBySlugFingerprint[slug] {
			if row.Phase == "queued" || row.Phase == "routing_rollback_queued" {
				sum.InflightCount++
			}
		}
		out[slug] = sum
	}
	return out
}

func repairPhaseLabel(mode, phase string) string {
	mode = strings.TrimSpace(mode)
	switch strings.TrimSpace(phase) {
	case "routing_forced_failed_detected":
		return "forced chain failed"
	case "routing_force_exhausted_detected":
		return "forced chain exhausted"
	case "routing_unstable_detected":
		return "unstable routing"
	case "attempts_exhausted":
		return "repair exhausted"
	case "queued":
		if mode == "routing_unstable" {
			return "unstable routing"
		}
		if mode == "degraded" {
			return "doctor queued"
		}
		if mode == "routing" {
			return "routing queued"
		}
		return "repair queued"
	case "routing_rollback_queued":
		return "rollback queued"
	case "completed":
		if mode == "degraded" {
			return "doctor repaired"
		}
		if mode == "routing" {
			return "routing stabilized"
		}
		return "repaired"
	case "routing_rollback_completed":
		return "rolled back"
	case "failed":
		if mode == "degraded" {
			return "doctor failed"
		}
		if mode == "routing" {
			return "routing failed"
		}
		return "repair failed"
	case "routing_rollback_failed":
		return "rollback failed"
	case "escalation_answered":
		return "manager answered"
	case "resolution_applied":
		return "manager applied"
	case "escalation_woke":
		return "manager woke"
	case "escalation_skipped":
		return "wake skipped"
	case "escalation_failed":
		return "wake failed"
	case "resolution_failed":
		return "resolution failed"
	case "delegation_queued":
		return "delegation queued"
	case "delegation_woke":
		return "delegation woke"
	case "delegation_failed":
		return "delegation failed"
	default:
		if strings.TrimSpace(phase) == "" {
			return "idle"
		}
		return phase
	}
}

func repairRowsView(rows []agentRepairRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, repairRowView(row))
	}
	return out
}

func repairRowView(row agentRepairRow) map[string]any {
	return map[string]any{
		"seq":                               row.Seq,
		"ts_unix_ms":                        row.TSUnixMS,
		"correlation_id":                    row.CorrelationID,
		"mode":                              row.Mode,
		"phase":                             row.Phase,
		"reason":                            row.Reason,
		"fingerprint":                       row.Fingerprint,
		"self_repair_attempt":               row.SelfRepairAttempt,
		"self_repair_max_attempts":          row.SelfRepairMaxAttempts,
		"issues":                            row.Issues,
		"applied":                           row.Applied,
		"answer":                            row.Answer,
		"error":                             row.Error,
		"target_agent":                      row.TargetAgent,
		"target_correlation":                row.TargetCorr,
		"mailbox_message_id":                row.MailboxMessage,
		"resolution":                        row.Resolution,
		"resolution_summary":                row.ResolutionSummary,
		"delegate_to":                       row.DelegateTo,
		"delegated_by":                      row.DelegatedBy,
		"root_agent":                        row.RootAgent,
		"chain_depth":                       row.ChainDepth,
		"incident_id":                       row.IncidentID,
		"root_incident_id":                  row.RootIncidentID,
		"parent_incident_id":                row.ParentIncidentID,
		"next_eligible_ms":                  row.NextEligibleMS,
		"routing_task_type":                 row.RoutingTaskType,
		"routing_task_model_chain":          row.RoutingTaskModelChain,
		"previous_routing_task_model_chain": row.PreviousRoutingTaskModelChain,
		"routing_force_generation":          row.RoutingForceGeneration,
		"previous_routing_force_generation": row.PreviousRoutingForceGeneration,
	}
}

func (s *Server) agentEscalationRows(st *board.Store, slug string, limit int, cursorTS int64, cursorID string) ([]agentEscalationRow, string) {
	msgs := st.Read("help", boardReadMaxLimit)
	metaByMessage := map[string]agentRepairRow{}
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "doctor.auto_repair" || e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		if plString(pl, "target_agent") != slug {
			return nil
		}
		msgID := plString(pl, "mailbox_message_id")
		if strings.TrimSpace(msgID) == "" {
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
			Error:                          plString(pl, "error"),
			Fingerprint:                    plString(pl, "fingerprint"),
			SelfRepairAttempt:              plInt(pl, "self_repair_attempt"),
			SelfRepairMaxAttempts:          plInt(pl, "self_repair_max_attempts"),
			TargetAgent:                    plString(pl, "target_agent"),
			TargetCorr:                     plString(pl, "target_correlation"),
			Resolution:                     plString(pl, "resolution"),
			ResolutionSummary:              plString(pl, "resolution_summary"),
			DelegateTo:                     plString(pl, "delegate_to"),
			DelegatedBy:                    plString(pl, "delegated_by"),
			RootAgent:                      plString(pl, "root_agent"),
			ChainDepth:                     intNumber(pl["chain_depth"]),
			IncidentID:                     plString(pl, "incident_id"),
			RootIncidentID:                 plString(pl, "root_incident_id"),
			ParentIncidentID:               plString(pl, "parent_incident_id"),
			RoutingForceGeneration:         intNumber(pl["routing_force_generation"]),
			PreviousRoutingForceGeneration: intNumber(pl["previous_routing_force_generation"]),
		}
		if cur, ok := metaByMessage[msgID]; !ok || row.Seq > cur.Seq {
			metaByMessage[msgID] = row
		}
		return nil
	})
	out := make([]agentEscalationRow, 0, len(msgs))
	for _, msg := range msgs {
		if !msg.Help {
			continue
		}
		if msg.To != slug && msg.To != board.Everyone {
			continue
		}
		replies := st.Replies(msg.ID, boardReadMaxLimit)
		acked := boardMessageAckedBy(msg, slug)
		status := "open"
		if len(replies) > 0 {
			status = "answered"
		} else if acked {
			status = "acked"
		}
		row := agentEscalationRow{
			MessageID:  msg.ID,
			From:       msg.From,
			To:         msg.To,
			Text:       msg.Text,
			TSUnixMS:   msg.TSMS,
			Status:     status,
			ReplyCount: len(replies),
			Acked:      acked,
		}
		if meta, ok := metaByMessage[msg.ID]; ok {
			row.SourceAgent = meta.Agent
			row.Mode = meta.Mode
			row.WakePhase = meta.Phase
			row.WakeReason = meta.Reason
			row.WakeError = meta.Error
			row.WakeCorrelationID = meta.TargetCorr
			row.Fingerprint = meta.Fingerprint
			row.Resolution = meta.Resolution
			row.ResolutionSummary = meta.ResolutionSummary
			row.DelegateTo = meta.DelegateTo
			row.RootAgent = meta.RootAgent
			row.ChainDepth = meta.ChainDepth
			row.IncidentID = meta.IncidentID
			row.RootIncidentID = meta.RootIncidentID
			row.ParentIncidentID = meta.ParentIncidentID
			if strings.HasPrefix(meta.Phase, "delegation_") {
				row.OriginKind = "delegated"
				row.OriginAgent = firstNonEmpty(meta.DelegatedBy, msg.From)
			} else {
				row.OriginKind = "doctor"
				row.OriginAgent = firstNonEmpty(msg.From, meta.DelegatedBy)
			}
		}
		// Prefer parsing the broken/source agent out of the message text only when
		// the event envelope didn't carry one (older history).
		if row.SourceAgent == "" {
			row.SourceAgent = escalationSourceFromText(msg.Text)
		}
		if row.RootAgent == "" {
			row.RootAgent = row.SourceAgent
		}
		if row.OriginKind == "" {
			row.OriginKind = "doctor"
			row.OriginAgent = msg.From
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TSUnixMS > out[j].TSUnixMS })
	// Cursor pagination: cursor encodes (TSUnixMS, MessageID) of the LAST
	// entry on the previous page; server skips entries strictly newer-or-equal.
	if cursorTS > 0 || cursorID != "" {
		filtered := out[:0]
		for _, r := range out {
			if r.TSUnixMS > cursorTS {
				continue
			}
			if r.TSUnixMS == cursorTS && r.MessageID >= cursorID {
				continue
			}
			filtered = append(filtered, r)
		}
		out = filtered
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
		last := out[limit-1]
		return out, strconv.FormatInt(last.TSUnixMS, 10) + ":" + last.MessageID
	}
	return out, ""
}

func boardMessageAckedBy(m board.Message, slug string) bool {
	slug = strings.ToLower(strings.TrimSpace(slug))
	for _, by := range m.AckedBy {
		if strings.ToLower(strings.TrimSpace(by)) == slug {
			return true
		}
	}
	return false
}

func escalationSourceFromText(text string) string {
	text = strings.TrimSpace(text)
	const prefix = "Doctor "
	if !strings.HasPrefix(text, prefix) {
		return ""
	}
	if i := strings.Index(text, " for agent "); i > 0 {
		// The agent named after "for agent" is the broken agent, not the owner.
		start := i + len(" for agent ")
		if end := strings.Index(text[start:], "."); end > 0 {
			return strings.TrimSpace(text[start : start+end])
		}
	}
	return ""
}

func joinActivityParts(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, " · ")
}

func wakeRunbookActivitySuffix(pl map[string]any) string {
	raw, _ := pl["autonomy_runbook"].(map[string]any)
	if len(raw) == 0 {
		return ""
	}
	parts := []string{
		plString(raw, "trigger_contract"),
		plString(raw, "route_contract"),
		plString(raw, "recovery_contract"),
		plString(raw, "sleep_contract"),
	}
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			clean = append(clean, part)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	return "contract " + strings.Join(clean, "/")
}

func (s *Server) agentWorkspaceInfo(p roster.Profile) (string, bool) {
	workdir := strings.TrimSpace(p.Workdir)
	if workdir == "" {
		return "", false
	}
	root := s.agentWorkspaceRoot()
	dir, ok := confineUnder(root, workdir)
	if !ok || filepath.Clean(dir) == filepath.Clean(root) {
		return "", false
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", false
	}
	files, bytes := countTreeFiles(dir)
	return filepath.ToSlash(workdir) + fmt.Sprintf(" (%d file(s), %d bytes)", files, bytes), true
}

func (s *Server) agentWorkspaceRoot() string {
	if ws := os.Getenv(brand.EnvPrefix + "WORKSPACE"); strings.TrimSpace(ws) != "" {
		return ws
	}
	return filepath.Join(s.k.BaseDir(), "workspace")
}

func countTreeFiles(root string) (int, int64) {
	var files int
	var bytes int64
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		files++
		bytes += info.Size()
		return nil
	})
	return files, bytes
}

func registerRosterCommands() {
	register(
		commandSpec{Cmd: CmdAgentList, Handler: func(dc *DispatchCtx) { dc.S.handleAgentList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentAdd, Handler: func(dc *DispatchCtx) { dc.S.handleAgentAdd(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentEdit, Handler: func(dc *DispatchCtx) { dc.S.handleAgentEdit(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentSetEnabled, Handler: func(dc *DispatchCtx) { dc.S.handleAgentSetEnabled(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRemove, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRemove(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentTaskUpdate, Handler: func(dc *DispatchCtx) { dc.S.handleAgentTaskUpdate(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentImpact, Handler: func(dc *DispatchCtx) { dc.S.handleAgentImpact(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentTombstone, Handler: func(dc *DispatchCtx) { dc.S.handleAgentTombstone(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentGraveyard, Handler: func(dc *DispatchCtx) { dc.S.handleAgentGraveyard(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentActivity, Handler: func(dc *DispatchCtx) { dc.S.handleAgentActivity(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRepairStatus, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRepairStatus(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRepair, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRepair(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentEscalations, Handler: func(dc *DispatchCtx) { dc.S.handleAgentEscalations(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentWake, Handler: func(dc *DispatchCtx) { dc.S.handleAgentWake(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentResolve, Handler: func(dc *DispatchCtx) { dc.S.handleAgentResolve(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRetire, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRetire(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRevive, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRevive(dc.Conn, dc.Req) }},
	)
}
