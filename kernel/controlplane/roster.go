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
	"errors"
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
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/plugins/tools/overseertool"
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

// agentModelChain builds a named agent's run chain: the resolved primary model
// first, then the profile's ordered fallbacks, skipping duplicates of the
// primary (so an explicit --model equal to a fallback doesn't try it twice).
func agentModelChain(primary string, fallbacks []string) []string {
	chain := []string{primary}
	for _, m := range fallbacks {
		if m = strings.TrimSpace(m); m != "" && m != primary {
			chain = append(chain, m)
		}
	}
	return chain
}

// profileView is the stable wire shape for one profile.
func profileView(p roster.Profile) map[string]any {
	b, _ := json.Marshal(p)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	m["kind"] = p.Kind()
	m["managed"] = !p.AllowsDirectCall()
	return m
}

func (s *Server) handleAgentList(conn net.Conn, req Request) {
	profiles := s.k.Roster().List()
	statuses := s.agentStatusViews(profiles)
	out := make([]any, 0, len(profiles))
	enabled := 0
	for _, p := range profiles {
		view := profileView(p)
		if st, ok := statuses[p.Slug]; ok {
			view["status"] = st
		}
		out = append(out, view)
		if p.Enabled {
			enabled++
		}
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"profiles": out, "count": len(out), "enabled_count": enabled},
	})
}

func (s *Server) agentStatusViews(profiles []roster.Profile) map[string]map[string]any {
	const reaperWindow = 30 * 24 * time.Hour
	const routingWindow = 24 * time.Hour
	cut := time.Now().Add(-reaperWindow).UnixMilli()
	routingCut := time.Now().Add(-routingWindow).UnixMilli()
	rep := s.k.ReaperScan(cut, cut)
	repairs := s.agentRepairSummaries()
	routingCounts := s.agentRoutingPressureViews(profiles, routingCut)
	retryCounts := s.agentRetryPressureViews(profiles, routingCut)
	escalationLoads := s.agentEscalationLoadViews(profiles)
	wakeStatuses := s.agentWakeStatusViews(profiles)
	liveStatuses := s.agentLiveStatusViews(profiles)
	lastActivities := s.agentLastActivityViews(profiles)
	autonomyRunbooks := s.agentLastAutonomyRunbookViews(profiles)
	mailboxWakes := s.agentMailboxWakeViews(profiles)
	policyDenials := s.agentPolicyDenialViews(profiles)

	degradedBySlug := map[string]runtime.DegradedAgent{}
	for _, row := range rep.DegradedAgents {
		degradedBySlug[row.Slug] = row
	}
	misconfiguredBySlug := map[string]runtime.MisconfiguredAgent{}
	for _, row := range rep.MisconfiguredAgents {
		misconfiguredBySlug[row.Slug] = row
	}
	routingBySlug := map[string]runtime.RoutingPressureAgent{}
	for _, row := range rep.RoutingPressure {
		routingBySlug[row.Slug] = row
	}
	forcedBySlug := map[string]runtime.RoutingForcedProbationAgent{}
	for _, row := range rep.RoutingForced {
		forcedBySlug[row.Slug] = row
	}
	forcedFailedBySlug := map[string]runtime.RoutingForcedFailedAgent{}
	for _, row := range rep.RoutingForcedFailed {
		forcedFailedBySlug[row.Slug] = row
	}
	forcedExhaustedBySlug := map[string]runtime.RoutingForcedExhaustedAgent{}
	for _, row := range rep.RoutingForcedExhausted {
		forcedExhaustedBySlug[row.Slug] = row
	}
	unstableBySlug := map[string]runtime.RoutingUnstableAgent{}
	for _, row := range rep.RoutingUnstable {
		unstableBySlug[row.Slug] = row
	}
	deadBySlug := map[string]runtime.ReaperAgent{}
	for _, row := range rep.DeadAgents {
		deadBySlug[row.Slug] = row
	}

	out := make(map[string]map[string]any, len(profiles))
	for _, p := range profiles {
		st := map[string]any{
			"health_state":               "healthy",
			"health_label":               "healthy",
			"repair_state":               "idle",
			"repair_label":               "idle",
			"invalid_runtime_overrides":  0,
			"misconfiguration_count":     0,
			"repair_inflight":            0,
			"self_repair_enabled":        p.SelfRepairPolicy != nil && p.SelfRepairPolicy.Enabled,
			"repair_next_eligible_ms":    int64(0),
			"repair_last_ts_ms":          int64(0),
			"repair_last_correlation_id": "",
			"routing_fallback_count":     0,
			"retry_count":                0,
			"escalation_open_count":      0,
			"escalation_acked_count":     0,
			"active_run_count":           0,
			"operational_state":          "sleeping",
			"operational_label":          "sleeping",
		}
		if p.Retired {
			st["health_state"] = "retired"
			st["health_label"] = "graveyard"
			st["operational_state"] = "retired"
			st["operational_label"] = "graveyard"
		} else if !p.Enabled {
			st["operational_state"] = "paused"
			st["operational_label"] = "paused"
		} else if row, ok := degradedBySlug[p.Slug]; ok {
			st["health_state"] = "degraded"
			st["health_label"] = "degraded"
			st["health_failures"] = row.Failures
			st["health_threshold"] = row.Threshold
			st["health_window"] = row.Window
			st["last_failure_ms"] = row.LastFailureMS
		} else if row, ok := misconfiguredBySlug[p.Slug]; ok {
			st["health_state"] = "misconfigured"
			st["health_label"] = "misconfigured"
			st["invalid_runtime_overrides"] = len(row.Issues)
			st["misconfiguration_count"] = len(row.Issues)
		} else if row, ok := forcedExhaustedBySlug[p.Slug]; ok {
			st["health_state"] = "force_exhausted"
			st["health_label"] = "forced chain exhausted"
			st["routing_fallback_count"] = row.Count
			st["routing_task_type"] = row.TaskType
			st["routing_forced_chain"] = row.ForcedChain
			st["routing_force_generation"] = row.ForceGeneration
		} else if row, ok := forcedFailedBySlug[p.Slug]; ok {
			st["health_state"] = "force_failed"
			st["health_label"] = "forced chain failed"
			st["routing_fallback_count"] = row.Count
			st["routing_task_type"] = row.TaskType
			st["routing_forced_chain"] = row.ForcedChain
			st["routing_force_generation"] = row.ForceGeneration
		} else if row, ok := unstableBySlug[p.Slug]; ok {
			st["health_state"] = "unstable"
			st["health_label"] = "unstable routing"
			st["routing_fallback_count"] = row.Count
			st["routing_task_type"] = row.TaskType
			st["routing_current_chain"] = row.CurrentChain
			st["routing_previous_chain"] = row.PreviousChain
		} else if row, ok := forcedBySlug[p.Slug]; ok {
			st["health_state"] = "stabilizing"
			st["health_label"] = "forced-chain probation"
			st["routing_fallback_count"] = row.Count
			st["routing_task_type"] = row.TaskType
			st["routing_forced_chain"] = row.ForcedChain
			st["routing_force_generation"] = row.ForceGeneration
		} else if row, ok := routingBySlug[p.Slug]; ok {
			st["health_state"] = "degraded"
			st["health_label"] = "fallback pressure"
			st["routing_fallback_count"] = row.Count
		} else if row, ok := deadBySlug[p.Slug]; ok {
			st["health_state"] = "stale"
			st["health_label"] = "stale"
			st["last_active_ms"] = row.LastActiveMS
		}
		if row, ok := misconfiguredBySlug[p.Slug]; ok && len(row.Issues) > 0 {
			st["invalid_runtime_overrides"] = len(row.Issues)
			st["misconfiguration_count"] = len(row.Issues)
			st["config_issues"] = row.Issues
		}
		if sum, ok := repairs[p.Slug]; ok {
			st["repair_inflight"] = sum.InflightCount
			if sum.HasLatest {
				st["repair_mode"] = sum.Latest.Mode
				st["repair_state"] = sum.Latest.Phase
				st["repair_label"] = repairPhaseLabel(sum.Latest.Mode, sum.Latest.Phase)
				st["repair_next_eligible_ms"] = sum.Latest.NextEligibleMS
				st["repair_last_ts_ms"] = sum.Latest.TSUnixMS
				st["repair_last_correlation_id"] = sum.Latest.CorrelationID
				st["repair_self_attempt"] = sum.Latest.SelfRepairAttempt
				st["repair_self_max_attempts"] = sum.Latest.SelfRepairMaxAttempts
				st["repair_incident_id"] = sum.Latest.IncidentID
				st["repair_root_incident_id"] = sum.Latest.RootIncidentID
				st["repair_parent_incident_id"] = sum.Latest.ParentIncidentID
				st["repair_root_agent"] = sum.Latest.RootAgent
				st["repair_chain_depth"] = sum.Latest.ChainDepth
				if sum.Latest.Error != "" {
					st["repair_last_error"] = sum.Latest.Error
				}
			}
		}
		if pressure, ok := routingCounts[p.Slug]; ok && pressure.Count > 0 {
			st["routing_fallback_count"] = pressure.Count
			st["routing_last_reason"] = pressure.LastReason
			st["routing_last_failed"] = pressure.LastFailed
			st["routing_last_next"] = pressure.LastNext
			st["routing_last_ts_ms"] = pressure.LastTSMS
		}
		if retry, ok := retryCounts[p.Slug]; ok && retry.Count > 0 {
			st["retry_count"] = retry.Count
			st["retry_last_reason"] = retry.LastReason
			st["retry_last_ts_ms"] = retry.LastTSMS
			st["retry_next_attempt"] = retry.NextAttempt
			st["retry_max_attempts"] = retry.MaxAttempts
		}
		if load, ok := escalationLoads[p.Slug]; ok {
			st["escalation_open_count"] = load.Open
			st["escalation_acked_count"] = load.Acked
		}
		if wake, ok := wakeStatuses[p.Slug]; ok {
			st["wake_schedule_count"] = wake.ScheduleCount
			st["wake_standing_count"] = wake.StandingCount
			st["wake_event_subjects"] = wake.EventSubjects
			st["next_wake_ms"] = wake.NextScheduledWakeMS
			st["next_wake_label"] = wake.NextScheduledLabel
		}
		if live, ok := liveStatuses[p.Slug]; ok {
			st["active_run_count"] = live.ActiveRuns
			st["active_correlation_id"] = live.ActiveCorrelationID
			st["active_intent"] = live.ActiveIntent
			st["active_started_ms"] = live.ActiveStartedMS
			st["active_model"] = live.ActiveModel
			st["active_spent_mc"] = live.ActiveSpentMc
			st["active_phase"] = live.ActivePhase
			st["active_last_event_ms"] = live.ActiveLastEventMS
			st["active_last_event_kind"] = live.ActiveLastEventKind
			st["active_detail"] = live.ActiveDetail
			st["active_tool"] = live.ActiveTool
			st["active_iter"] = live.ActiveIter
			st["active_wake_source"] = live.ActiveWakeSource
			st["active_wake_reason"] = live.ActiveWakeReason
			st["active_schedule_id"] = live.ActiveScheduleID
			st["active_standing_id"] = live.ActiveStandingID
			st["active_standing_name"] = live.ActiveStandingName
			st["active_trigger_subject"] = live.ActiveTriggerSubject
			st["active_parent_correlation"] = live.ActiveParentCorrelation
			if live.ActiveRuns > 0 {
				st["operational_state"] = "running"
				st["operational_label"] = live.ActivePhase
				if live.ActivePhase == "" {
					st["operational_label"] = "running"
				}
			}
		}
		if last, ok := lastActivities[p.Slug]; ok {
			st["last_activity_ms"] = last.TSUnixMS
			st["last_activity_kind"] = last.Kind
			st["last_activity_correlation_id"] = last.CorrelationID
			st["last_activity_summary"] = last.Summary
		}
		if runbook, ok := autonomyRunbooks[p.Slug]; ok {
			st["last_autonomy_runbook"] = runbook
		}
		if mw, ok := mailboxWakes[p.Slug]; ok && len(mw) > 0 {
			st["mailbox_wakes"] = mw
		}
		if d, ok := policyDenials[p.Slug]; ok && d.Count > 0 {
			st["policy_denied_count"] = d.Count
			st["policy_denied_last_tool"] = d.LastTool
			st["policy_denied_last_reason"] = d.LastReason
			st["policy_denied_last_capability"] = d.LastCapability
			st["policy_denied_last_hard"] = d.LastHard
			st["policy_denied_last_ms"] = d.LastTSMS
		}
		out[p.Slug] = st
	}
	return out
}

// isMailboxWakeSubject reports whether a standing trigger subject is the mailbox
// wake route — a board.posted subject (board.dm.<slug> / board.help[.<slug>] /
// board.broadcast / board.<topic>). The board notifier routes every door's write
// onto one of these subjects, so a standing order matching one is a message wake.
func isMailboxWakeSubject(subject string) bool {
	return subject == "board" || strings.HasPrefix(subject, "board.")
}

func (s *Server) agentLastAutonomyRunbookViews(profiles []roster.Profile) map[string]map[string]any {
	if len(profiles) == 0 {
		return nil
	}
	known := make(map[string]bool, len(profiles))
	for _, p := range profiles {
		known[p.Slug] = true
	}
	out := map[string]map[string]any{}
	_ = s.k.Journal().Range(func(e *event.Event) error {
		isDoctorWake := e.Subject == "doctor.auto_repair" && e.Kind == event.KindInfo
		if !((e.Subject == "agent.wake" && e.Kind == event.KindInfo) || e.Kind == event.KindScheduleFired || e.Kind == event.KindStandingFired || e.Kind == event.KindSubAgentSpawned || isDoctorWake) {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		// A doctor escalation/delegation wakes the OWNER/delegate named in
		// target_agent (the "agent" field is the escalating agent whose incident
		// triggered the wake). Only the escalation_woke/delegation_woke phases carry
		// an autonomy_runbook, so the runbook guard below filters the other phases.
		slug := plString(pl, "agent")
		if isDoctorWake {
			slug = plString(pl, "target_agent")
		}
		if !known[slug] {
			return nil
		}
		raw, ok := pl["autonomy_runbook"].(map[string]any)
		if !ok || len(raw) == 0 {
			return nil
		}
		cp := map[string]any{}
		for k, v := range raw {
			cp[k] = v
		}
		phase := plString(pl, "phase")
		switch {
		case phase == "" && e.Kind == event.KindScheduleFired:
			phase = "schedule_fired"
		case phase == "" && e.Kind == event.KindStandingFired:
			phase = "standing_fired"
		case phase == "" && e.Kind == event.KindSubAgentSpawned:
			phase = "delegated_wake"
		}
		cp["phase"] = phase
		switch {
		case e.Kind == event.KindScheduleFired:
			cp["source"] = "schedule"
			cp["schedule_id"] = plString(pl, "schedule_id")
		case e.Kind == event.KindStandingFired:
			// standing.fired payloads address the order by "id"/"name" (not the
			// schedule's "standing_id"/"standing_name"); accept either form.
			cp["source"] = "standing"
			cp["standing_id"] = firstNonEmpty(plString(pl, "standing_id"), plString(pl, "id"))
			if name := firstNonEmpty(plString(pl, "standing_name"), plString(pl, "name")); name != "" {
				cp["standing_name"] = name
			}
			subj := plString(pl, "trigger_subject")
			if subj != "" {
				cp["trigger_subject"] = subj
			}
			// A standing order whose event trigger matched a board subject IS the
			// mailbox-wake route (board.dm.<slug> / board.help / board.broadcast /
			// board.<topic>). Enrich the runbook with which message woke the agent
			// and from whom, so message -> wake event -> run correlation is
			// traceable (the event's CorrelationID is the run correlation).
			if isMailboxWakeSubject(subj) {
				cp["wake_via"] = "mailbox"
				tp, _ := pl["trigger_payload"].(map[string]any)
				if id := plString(tp, "id"); id != "" {
					cp["mailbox_message_id"] = id
				}
				if from := plString(tp, "from"); from != "" {
					cp["mailbox_from"] = from
				}
				if to := plString(tp, "to"); to != "" {
					cp["mailbox_to"] = to
				}
				if rt := plString(tp, "reply_to"); rt != "" {
					cp["mailbox_reply_to"] = rt
				}
				if help, ok := tp["help"].(bool); ok && help {
					cp["mailbox_help"] = true
				}
			}
		case e.Kind == event.KindSubAgentSpawned:
			// A delegated sub-agent spawn is journaled under the PARENT/lead
			// correlation; the child run is in child_correlation. Surface who
			// delegated it and the parent run so the wake is attributable up to the
			// leader, while correlation_id (below) points at the child run to drill
			// into.
			cp["source"] = "delegated"
			if by := plString(pl, "delegated_by"); by != "" {
				cp["delegated_by"] = by
			}
			if pc := firstNonEmpty(plString(pl, "parent_correlation_id"), plString(pl, "parent")); pc != "" {
				cp["parent_correlation_id"] = pc
			}
		case isDoctorWake:
			// Doctor woke this agent to handle another agent's incident: "agent" is
			// the agent being repaired (doctor_for); the event's CorrelationID is
			// this agent's own run. delegated_by is set on the delegation_woke hop.
			cp["source"] = "doctor"
			if forAgent := plString(pl, "agent"); forAgent != "" {
				cp["doctor_for"] = forAgent
			}
			if mode := plString(pl, "mode"); mode != "" {
				cp["doctor_mode"] = mode
			}
			if inc := plString(pl, "incident_id"); inc != "" {
				cp["incident_id"] = inc
			}
			if by := plString(pl, "delegated_by"); by != "" {
				cp["delegated_by"] = by
			}
		}
		corrID := e.CorrelationID
		if e.Kind == event.KindSubAgentSpawned {
			if child := plString(pl, "child_correlation"); child != "" {
				corrID = child
			}
		}
		cp["correlation_id"] = corrID
		cp["ts_unix_ms"] = e.TSUnixMS
		out[slug] = cp
		return nil
	})
	return out
}

// agentMailboxWakeViews maps, per agent, each board message id that actually woke
// the agent to the wake's run correlation and timestamp. It is the causality layer
// over the mailbox-wake route (a standing order matched on a board.* subject): the
// comms tab can then mark "this message woke the agent" and link the message to the
// run it triggered, instead of leaving the operator to infer it. Read-only and
// derived from the journal — stored board messages are never mutated. Latest wake
// per message id wins.
func (s *Server) agentMailboxWakeViews(profiles []roster.Profile) map[string]map[string]any {
	if len(profiles) == 0 {
		return nil
	}
	known := make(map[string]bool, len(profiles))
	for _, p := range profiles {
		known[p.Slug] = true
	}
	out := map[string]map[string]any{}
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindStandingFired {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		slug := plString(pl, "agent")
		if !known[slug] || !isMailboxWakeSubject(plString(pl, "trigger_subject")) {
			return nil
		}
		tp, _ := pl["trigger_payload"].(map[string]any)
		msgID := plString(tp, "id")
		if msgID == "" {
			return nil
		}
		byMsg := out[slug]
		if byMsg == nil {
			byMsg = map[string]any{}
			out[slug] = byMsg
		}
		// Journal Range is chronological, so a later fire overwrites an earlier one
		// for the same message id — the most recent wake wins.
		byMsg[msgID] = map[string]any{
			"correlation_id":  e.CorrelationID,
			"ts_unix_ms":      e.TSUnixMS,
			"trigger_subject": plString(pl, "trigger_subject"),
		}
		return nil
	})
	return out
}

func (s *Server) agentLastActivityViews(profiles []roster.Profile) map[string]agentLastActivity {
	if len(profiles) == 0 {
		return nil
	}
	known := make(map[string]bool, len(profiles))
	runCorr := make(map[string]map[string]bool, len(profiles))
	out := make(map[string]agentLastActivity, len(profiles))
	for _, p := range profiles {
		known[p.Slug] = true
		runCorr[p.Slug] = map[string]bool{}
	}
	_ = s.k.Journal().Range(func(e *event.Event) error {
		var pl map[string]any
		_ = json.Unmarshal(e.Payload, &pl)
		if e.Kind == event.KindTaskReceived {
			if slug := plString(pl, "agent"); slug != "" && known[slug] && e.CorrelationID != "" {
				runCorr[slug][e.CorrelationID] = true
			}
		}
		for slug := range known {
			summary, ok := agentActivitySummary(e, pl, slug, runCorr[slug])
			if !ok {
				continue
			}
			cur := out[slug]
			if e.TSUnixMS >= cur.TSUnixMS {
				out[slug] = agentLastActivity{
					TSUnixMS:      e.TSUnixMS,
					Kind:          string(e.Kind),
					CorrelationID: e.CorrelationID,
					Summary:       summary,
				}
			}
		}
		return nil
	})
	return out
}

type agentPolicyDenials struct {
	Count          int
	LastTool       string
	LastReason     string
	LastCapability string
	LastHard       bool
	LastTSMS       int64
}

// agentPolicyDenialViews folds the journal's policy.decision evidence into a
// per-agent denial summary so the console can show that the runtime actually
// ENFORCED policy (tool refused), not merely that the UI displays a tool_deny.
// policy.decision events carry no agent slug, so denials are attributed by run
// correlation: a KindTaskReceived names the agent for a correlation, and any
// allow==false policy.decision under that correlation counts against it. Single
// chronological pass — task.received precedes its run's policy decisions.
func (s *Server) agentPolicyDenialViews(profiles []roster.Profile) map[string]agentPolicyDenials {
	if len(profiles) == 0 {
		return nil
	}
	known := make(map[string]bool, len(profiles))
	for _, p := range profiles {
		known[p.Slug] = true
	}
	corrToSlug := map[string]string{}
	out := map[string]agentPolicyDenials{}
	_ = s.k.Journal().Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindTaskReceived:
			var pl map[string]any
			if json.Unmarshal(e.Payload, &pl) != nil {
				return nil
			}
			if slug := plString(pl, "agent"); slug != "" && known[slug] && e.CorrelationID != "" {
				corrToSlug[e.CorrelationID] = slug
			}
		case event.KindPolicyDecision:
			slug := corrToSlug[e.CorrelationID]
			if slug == "" {
				return nil
			}
			var pl map[string]any
			if json.Unmarshal(e.Payload, &pl) != nil {
				return nil
			}
			if allow, ok := pl["allow"].(bool); !ok || allow {
				return nil
			}
			d := out[slug]
			d.Count++
			d.LastTool = plString(pl, "tool")
			d.LastReason = plString(pl, "reason")
			d.LastCapability = plString(pl, "capability")
			d.LastHard, _ = pl["hard_denied"].(bool)
			d.LastTSMS = e.TSUnixMS
			out[slug] = d
		}
		return nil
	})
	return out
}

func (s *Server) agentLiveStatusViews(profiles []roster.Profile) map[string]agentLiveStatus {
	if len(profiles) == 0 {
		return nil
	}
	known := make(map[string]bool, len(profiles))
	for _, p := range profiles {
		known[p.Slug] = true
	}
	runs, err := s.collectRuns(s.k)
	if err != nil {
		return nil
	}
	out := make(map[string]agentLiveStatus)
	runAgent := make(map[string]string)
	for _, r := range runs {
		if runEntryStatus(r) != "running" || strings.TrimSpace(r.Agent) == "" || !known[r.Agent] {
			continue
		}
		runAgent[r.CorrelationID] = r.Agent
		row := out[r.Agent]
		row.ActiveRuns++
		if row.ActiveStartedMS == 0 || r.StartedUnixMS > row.ActiveStartedMS {
			row.ActiveCorrelationID = r.CorrelationID
			row.ActiveIntent = r.Intent
			row.ActiveStartedMS = r.StartedUnixMS
			row.ActiveModel = r.Model
			row.ActiveSpentMc = r.SpentMicrocents
			row.ActivePhase = "starting"
			row.ActiveLastEventMS = r.StartedUnixMS
			row.ActiveLastEventKind = string(event.KindTaskReceived)
			row.ActiveParentCorrelation = r.ParentCorrelation
		}
		out[r.Agent] = row
	}
	if len(runAgent) == 0 {
		return out
	}
	_ = s.k.Journal().Range(func(e *event.Event) error {
		agentSlug, ok := runAgent[e.CorrelationID]
		if !ok {
			return nil
		}
		row := out[agentSlug]
		if row.ActiveCorrelationID != e.CorrelationID {
			return nil
		}
		var pl map[string]any
		_ = json.Unmarshal(e.Payload, &pl)
		row = applyActiveWakeContext(row, e.Kind, pl)
		if e.TSUnixMS < row.ActiveLastEventMS {
			out[agentSlug] = row
			return nil
		}
		phase, detail, tool, iter := liveEventSummary(e.Kind, pl)
		if phase == "" {
			out[agentSlug] = row
			return nil
		}
		row.ActivePhase = phase
		row.ActiveDetail = detail
		row.ActiveTool = tool
		row.ActiveIter = iter
		row.ActiveLastEventMS = e.TSUnixMS
		row.ActiveLastEventKind = string(e.Kind)
		out[agentSlug] = row
		return nil
	})
	return out
}

func applyActiveWakeContext(row agentLiveStatus, kind event.Kind, pl map[string]any) agentLiveStatus {
	switch kind {
	case event.KindTaskReceived:
		row.ActiveWakeSource = firstNonEmpty(row.ActiveWakeSource, plString(pl, "wake_source"), plString(pl, "source"))
		row.ActiveWakeReason = firstNonEmpty(row.ActiveWakeReason, plString(pl, "wake_reason"), plString(pl, "reason"))
		row.ActiveScheduleID = firstNonEmpty(row.ActiveScheduleID, plString(pl, "schedule_id"))
		row.ActiveStandingID = firstNonEmpty(row.ActiveStandingID, plString(pl, "standing_id"))
		row.ActiveStandingName = firstNonEmpty(row.ActiveStandingName, plString(pl, "standing_name"))
		row.ActiveTriggerSubject = firstNonEmpty(row.ActiveTriggerSubject, plString(pl, "trigger_subject"), plString(pl, "event_subject"))
		row.ActiveParentCorrelation = firstNonEmpty(row.ActiveParentCorrelation, plString(pl, "parent_correlation"))
	case event.KindScheduleFired:
		row.ActiveWakeSource = firstNonEmpty(row.ActiveWakeSource, "schedule")
		row.ActiveWakeReason = firstNonEmpty(row.ActiveWakeReason, plString(pl, "target"))
		row.ActiveScheduleID = firstNonEmpty(row.ActiveScheduleID, plString(pl, "schedule_id"))
	case event.KindStandingFired:
		row.ActiveWakeSource = firstNonEmpty(row.ActiveWakeSource, "standing")
		row.ActiveWakeReason = firstNonEmpty(row.ActiveWakeReason, "event")
		row.ActiveStandingID = firstNonEmpty(row.ActiveStandingID, plString(pl, "standing_id"), plString(pl, "id"))
		row.ActiveStandingName = firstNonEmpty(row.ActiveStandingName, plString(pl, "standing_name"), plString(pl, "name"))
		row.ActiveTriggerSubject = firstNonEmpty(row.ActiveTriggerSubject, plString(pl, "trigger_subject"))
	}
	if row.ActiveParentCorrelation != "" && row.ActiveWakeSource == "" {
		row.ActiveWakeSource = "subagent"
	}
	return row
}

func liveEventSummary(kind event.Kind, pl map[string]any) (phase, detail, tool string, iter int) {
	iter = plInt(pl, "iter")
	switch kind {
	case event.KindTaskReceived:
		return "starting", truncate(plString(pl, "intent"), 100), "", iter
	case event.KindLLMRequest:
		model := plString(pl, "model")
		if model != "" {
			return "thinking", "model: " + model, "", iter
		}
		return "thinking", "", "", iter
	case event.KindLLMResponse:
		if plInt(pl, "tool_calls") > 0 {
			return "planning tools", "", "", iter
		}
		return "answering", "", "", iter
	case event.KindToolInvoked:
		tool = firstNonEmpty(plString(pl, "tool"), plString(pl, "name"))
		if tool != "" {
			return "using tool", tool, tool, iter
		}
		return "using tool", "", "", iter
	case event.KindToolResult:
		tool = firstNonEmpty(plString(pl, "tool"), plString(pl, "name"))
		if tool != "" {
			return "observing tool", tool, tool, iter
		}
		return "observing tool", "", "", iter
	case event.KindAgentRetry, event.KindProviderRetry:
		reason := firstNonEmpty(plString(pl, "reason"), plString(pl, "error"))
		return "retrying", truncate(reason, 100), "", iter
	case event.KindTaskContinued:
		return "continuing", "", "", iter
	case event.KindRunPaused:
		return "paused", "", "", iter
	case event.KindRunResumed:
		return "resumed", "", "", iter
	case event.KindRunSteered:
		return "steered", truncate(plString(pl, "directive"), 100), "", iter
	}
	return "", "", "", iter
}

func (s *Server) agentWakeStatusViews(profiles []roster.Profile) map[string]agentWakeStatus {
	if len(profiles) == 0 {
		return nil
	}
	out := make(map[string]agentWakeStatus, len(profiles))
	for _, p := range profiles {
		out[p.Slug] = agentWakeStatus{}
	}
	for _, e := range s.k.Schedules().List() {
		for _, p := range profiles {
			if !scheduleEntryMatchesAgent(e, p.Slug) {
				continue
			}
			row := out[p.Slug]
			row.ScheduleCount++
			if e.Enabled && e.NextRunUnix > 0 {
				nextMS := e.NextRunUnix * 1000
				if row.NextScheduledWakeMS == 0 || nextMS < row.NextScheduledWakeMS {
					row.NextScheduledWakeMS = nextMS
					row.NextScheduledLabel = scheduleWakeLabel(e)
				}
			}
			out[p.Slug] = row
		}
	}
	for _, o := range s.k.Standing().List() {
		agent := strings.TrimSpace(o.Agent)
		if agent == "" {
			continue
		}
		for _, p := range profiles {
			if !strings.EqualFold(agent, p.Slug) {
				continue
			}
			row := out[p.Slug]
			row.StandingCount++
			for _, t := range o.Triggers {
				if t.Type == standing.TriggerEvent && strings.TrimSpace(t.Subject) != "" {
					row.EventSubjects = append(row.EventSubjects, strings.TrimSpace(t.Subject))
				}
			}
			sort.Strings(row.EventSubjects)
			row.EventSubjects = uniqueStrings(row.EventSubjects)
			out[p.Slug] = row
		}
	}
	for slug, row := range out {
		if row.ScheduleCount == 0 && row.StandingCount == 0 {
			delete(out, slug)
		}
	}
	return out
}

func scheduleEntryMatchesAgent(e cadence.Entry, slug string) bool {
	if strings.EqualFold(strings.TrimSpace(e.Agent), slug) {
		return true
	}
	return strings.EqualFold(legacyScheduleAgentSlug(e.Intent), slug)
}

func legacyScheduleAgentSlug(intent string) string {
	fields := strings.Fields(strings.TrimSpace(intent))
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if strings.HasPrefix(f, "--agent=") {
			return strings.TrimSpace(strings.TrimPrefix(f, "--agent="))
		}
		if f == "--agent" && i+1 < len(fields) {
			return strings.TrimSpace(fields[i+1])
		}
	}
	return ""
}

func scheduleWakeLabel(e cadence.Entry) string {
	target := strings.TrimSpace(e.Target)
	switch target {
	case cadence.TargetWorkflow:
		if strings.TrimSpace(e.Workflow) != "" {
			return "workflow " + strings.TrimSpace(e.Workflow)
		}
	case cadence.TargetSystemTask:
		if strings.TrimSpace(e.SystemTask) != "" {
			return "system task " + strings.TrimSpace(e.SystemTask)
		}
	case cadence.TargetTool:
		if strings.TrimSpace(e.Tool) != "" {
			return "tool " + strings.TrimSpace(e.Tool)
		}
	}
	if strings.TrimSpace(e.Intent) != "" {
		return strings.TrimSpace(e.Intent)
	}
	return e.ID
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := in[:0]
	var prev string
	for _, s := range in {
		if s == "" || s == prev {
			continue
		}
		out = append(out, s)
		prev = s
	}
	return out
}

func (s *Server) agentEscalationLoadViews(profiles []roster.Profile) map[string]agentEscalationLoad {
	if len(profiles) == 0 {
		return nil
	}
	st, err := s.boardReader()
	if err != nil || st == nil {
		return nil
	}
	known := make(map[string]bool, len(profiles))
	out := make(map[string]agentEscalationLoad, len(profiles))
	for _, p := range profiles {
		known[strings.ToLower(strings.TrimSpace(p.Slug))] = true
	}
	for _, msg := range st.OpenHelp(boardReadMaxLimit) {
		to := strings.ToLower(strings.TrimSpace(msg.To))
		if to == "" {
			continue
		}
		if to == board.Everyone {
			for slug := range known {
				if strings.EqualFold(strings.TrimSpace(msg.From), slug) {
					continue
				}
				row := out[slug]
				if boardMessageAckedBy(msg, slug) {
					row.Acked++
				} else {
					row.Open++
				}
				out[slug] = row
			}
			continue
		}
		if !known[to] {
			continue
		}
		row := out[to]
		if boardMessageAckedBy(msg, to) {
			row.Acked++
		} else {
			row.Open++
		}
		out[to] = row
	}
	return out
}

func (s *Server) agentRoutingPressureViews(profiles []roster.Profile, cutoffMS int64) map[string]agentRoutingPressure {
	if len(profiles) == 0 {
		return nil
	}
	out := make(map[string]agentRoutingPressure, len(profiles))
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindProviderFallback || e.TSUnixMS < cutoffMS {
			return nil
		}
		var pl struct {
			FailedModel string `json:"failed_model"`
			NextModel   string `json:"next_model"`
			Reason      string `json:"reason"`
			Scope       string `json:"scope"`
			TaskType    string `json:"task_type"`
		}
		if json.Unmarshal(e.Payload, &pl) != nil || strings.TrimSpace(pl.Scope) != "model-chain" {
			return nil
		}
		for _, p := range profiles {
			if !agentRoutingMatchesProfile(p, pl.TaskType, pl.FailedModel, pl.NextModel) {
				continue
			}
			row := out[p.Slug]
			row.Count++
			if e.TSUnixMS >= row.LastTSMS {
				row.LastReason = strings.TrimSpace(pl.Reason)
				row.LastFailed = strings.TrimSpace(pl.FailedModel)
				row.LastNext = strings.TrimSpace(pl.NextModel)
				row.LastTSMS = e.TSUnixMS
			}
			out[p.Slug] = row
		}
		return nil
	})
	return out
}

func (s *Server) agentRetryPressureViews(profiles []roster.Profile, cutoffMS int64) map[string]agentRetryPressure {
	if len(profiles) == 0 {
		return nil
	}
	known := make(map[string]bool, len(profiles))
	for _, p := range profiles {
		known[p.Slug] = true
	}
	runAgent := map[string]string{}
	out := make(map[string]agentRetryPressure, len(profiles))
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if e.TSUnixMS < cutoffMS {
			return nil
		}
		var pl map[string]any
		_ = json.Unmarshal(e.Payload, &pl)
		if e.Kind == event.KindTaskReceived {
			if slug := plString(pl, "agent"); slug != "" && known[slug] && e.CorrelationID != "" {
				runAgent[e.CorrelationID] = slug
			}
			return nil
		}
		if e.Kind != event.KindAgentRetry {
			return nil
		}
		slug := plString(pl, "agent")
		if slug == "" {
			slug = runAgent[e.CorrelationID]
		}
		if !known[slug] {
			return nil
		}
		row := out[slug]
		row.Count++
		if e.TSUnixMS >= row.LastTSMS {
			row.LastReason = firstNonEmpty(plString(pl, "reason"), plString(pl, "error"))
			row.LastTSMS = e.TSUnixMS
			row.NextAttempt = plInt(pl, "next_attempt")
			row.MaxAttempts = plInt(pl, "max_attempts")
		}
		out[slug] = row
		return nil
	})
	return out
}

func agentRoutingMatchesProfile(p roster.Profile, taskType, failedModel, nextModel string) bool {
	taskType = strings.TrimSpace(taskType)
	failedModel = strings.TrimSpace(failedModel)
	nextModel = strings.TrimSpace(nextModel)
	if pt := strings.TrimSpace(p.TaskType); pt != "" && strings.EqualFold(pt, taskType) {
		return true
	}
	for _, model := range agentModelChain(strings.TrimSpace(p.Model), p.Fallbacks) {
		if strings.EqualFold(model, failedModel) || strings.EqualFold(model, nextModel) {
			return true
		}
	}
	return false
}

func (s *Server) handleAgentAdd(conn net.Conn, req Request) {
	raw, ok := req.Args["profile"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.profile required"})
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.profile: " + err.Error()})
		return
	}
	var p roster.Profile
	if err := json.Unmarshal(b, &p); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.profile: " + err.Error()})
		return
	}
	normalizeAgentProfileKind(b, &p)
	p.System = false // System is kernel-owned (set only by guardian seeding); never accept it from a client (M961)
	if err := s.validateAgentHierarchyRefs(p); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	saved, err := s.k.AddProfile(p)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"profile": profileView(saved)}})
}

// handleAgentEdit applies args.profile's MUTABLE fields wholesale to the
// profile named by args.ref (identity/lifecycle fields are protected by the
// store, so a stale client can't rename a slug or resurrect a paused agent).
func (s *Server) handleAgentEdit(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	raw, ok := req.Args["profile"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.profile required"})
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.profile: " + err.Error()})
		return
	}
	var in roster.Profile
	if err := json.Unmarshal(b, &in); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.profile: " + err.Error()})
		return
	}
	normalizeAgentProfileKind(b, &in)
	current, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	candidate := current
	applyAgentMutableProfileFields(&candidate, in)
	if err := s.validateAgentHierarchyRefs(candidate); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	p, found, err := s.k.UpdateProfile(ref, func(dst *roster.Profile) {
		applyAgentMutableProfileFields(dst, in)
	})
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"profile": profileView(p)}})
}

func applyAgentMutableProfileFields(dst *roster.Profile, in roster.Profile) {
	dst.Name = in.Name
	dst.Soul = in.Soul
	dst.Instructions = in.Instructions
	dst.Model = in.Model
	dst.Fallbacks = in.Fallbacks
	dst.TaskType = in.TaskType
	dst.MaxCostMc = in.MaxCostMc
	dst.MaxDailyMc = in.MaxDailyMc
	dst.MemoryScope = in.MemoryScope
	dst.Workdir = in.Workdir
	dst.OwnerAgent = in.OwnerAgent
	dst.ParentAgent = in.ParentAgent
	dst.DirectCallable = in.DirectCallable
	dst.RetryPolicy = in.RetryPolicy
	dst.HealthPolicy = in.HealthPolicy
	dst.SelfRepairPolicy = in.SelfRepairPolicy
	dst.NoisePolicy = in.NoisePolicy
	dst.ToolAllow = in.ToolAllow
	dst.ToolDeny = in.ToolDeny
	dst.TrustCeiling = in.TrustCeiling
	dst.ConfigOverrides = in.ConfigOverrides
	dst.Lifecycle = in.Lifecycle
	dst.TaskList = in.TaskList
	dst.Description = in.Description
}

func normalizeAgentProfileKind(raw []byte, p *roster.Profile) {
	var meta struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return
	}
	if strings.EqualFold(strings.TrimSpace(meta.Kind), "subagent") {
		no := false
		p.DirectCallable = &no
	}
}

func (s *Server) validateAgentHierarchyRefs(p roster.Profile) error {
	for label, ref := range map[string]string{"owner_agent": p.OwnerAgent, "parent_agent": p.ParentAgent} {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if strings.EqualFold(ref, strings.TrimSpace(p.Slug)) {
			return fmt.Errorf("roster: %s cannot point to the same agent", label)
		}
		target, ok := s.k.Roster().Get(ref)
		if !ok {
			return fmt.Errorf("roster: %s %q does not exist", label, ref)
		}
		if target.Retired {
			return fmt.Errorf("roster: %s %q is retired", label, ref)
		}
	}
	return nil
}

func managedSubagentDirectCallError(p roster.Profile, action string) string {
	manager := strings.TrimSpace(p.ParentAgent)
	if manager == "" {
		manager = strings.TrimSpace(p.OwnerAgent)
	}
	hint := "route the work through its parent/owner agent"
	if manager != "" {
		hint = "wake " + manager + " or delegate through it"
	}
	return "agent " + p.Slug + " is a managed sub-agent and cannot be " + action + " directly; " + hint
}

func (s *Server) handleAgentSetEnabled(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	// Accept enabled as a bool (CLI/JSON) or a "true"/"false"/"1"/"0" string
	// (the webui query-arg transport carries every value as a string).
	enabled := false
	switch v := req.Args["enabled"].(type) {
	case bool:
		enabled = v
	case string:
		enabled = strings.EqualFold(v, "true") || v == "1"
	}
	p, err := s.k.SetProfileEnabled(ref, enabled)
	if err != nil {
		if errors.Is(err, roster.ErrNotFound) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
			return
		}
		if errors.Is(err, roster.ErrRetired) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + ref + " is retired — revive it first"})
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	res := map[string]any{"profile": profileView(p)}
	if enabled {
		res["standing_paused"] = s.countAgentPausedStanding(p.Slug)
		res["schedules_paused"] = s.countAgentPausedSchedules(p.Slug)
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: res})
}

func (s *Server) handleAgentTaskUpdate(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	op, _ := req.Args["op"].(string)
	op = strings.ToLower(strings.TrimSpace(op))
	if op == "" {
		op = "update"
	}
	if op != "add" && op != "update" && op != "remove" && op != "delete" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.op must be add, update, or remove"})
		return
	}
	var in roster.AgentTask
	if raw, ok := req.Args["task"]; ok {
		b, err := json.Marshal(raw)
		if err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.task: " + err.Error()})
			return
		}
		if err := json.Unmarshal(b, &in); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.task: " + err.Error()})
			return
		}
	}
	if v, ok := req.Args["id"].(string); ok {
		in.ID = v
	}
	if v, ok := req.Args["title"].(string); ok {
		in.Title = v
	}
	if v, ok := req.Args["description"].(string); ok {
		in.Description = v
	}
	if v, ok := req.Args["scope"].(string); ok {
		in.Scope = v
	}
	if v, ok := req.Args["status"].(string); ok {
		in.Status = v
	}
	titleProvided := hasArg(req.Args, "title") || taskFieldPresent(req.Args["task"], "title")
	scopeProvided := hasArg(req.Args, "scope") || taskFieldPresent(req.Args["task"], "scope")
	statusProvided := hasArg(req.Args, "status") || taskFieldPresent(req.Args["task"], "status")
	if op == "add" || (op == "update" && titleProvided) {
		if strings.TrimSpace(in.Title) == "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.title required"})
			return
		}
	}
	if scopeProvided {
		switch strings.TrimSpace(in.Scope) {
		case "", "cycle", "total":
		default:
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.scope must be cycle or total"})
			return
		}
	}
	if statusProvided {
		switch strings.TrimSpace(in.Status) {
		case "", "todo", "doing", "done", "blocked", "retired":
		default:
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.status must be todo, doing, done, blocked, or retired"})
			return
		}
	}
	var task roster.AgentTask
	found := false
	p, exists, err := s.k.UpdateProfile(ref, func(dst *roster.Profile) {
		switch op {
		case "add":
			task = in
			dst.TaskList = append(dst.TaskList, task)
			found = true
		case "update":
			id := strings.TrimSpace(in.ID)
			if id == "" {
				return
			}
			for i := range dst.TaskList {
				if dst.TaskList[i].ID != id {
					continue
				}
				if _, ok := req.Args["title"]; ok || in.Title != "" {
					dst.TaskList[i].Title = in.Title
				}
				if _, ok := req.Args["description"]; ok || in.Description != "" {
					dst.TaskList[i].Description = in.Description
				}
				if _, ok := req.Args["scope"]; ok || in.Scope != "" {
					dst.TaskList[i].Scope = in.Scope
				}
				if _, ok := req.Args["status"]; ok || in.Status != "" {
					dst.TaskList[i].Status = in.Status
				}
				task = dst.TaskList[i]
				found = true
				return
			}
		case "remove", "delete":
			id := strings.TrimSpace(in.ID)
			if id == "" {
				return
			}
			for i := range dst.TaskList {
				if dst.TaskList[i].ID != id {
					continue
				}
				task = dst.TaskList[i]
				dst.TaskList = append(append([]roster.AgentTask{}, dst.TaskList[:i]...), dst.TaskList[i+1:]...)
				found = true
				return
			}
		}
	})
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if !exists {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	if !found {
		if strings.TrimSpace(in.ID) == "" && op != "add" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
			return
		}
		if op == "add" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.title required"})
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent task: " + in.ID})
		return
	}
	if op == "add" {
		for _, t := range p.TaskList {
			if t.Title == strings.TrimSpace(in.Title) && (strings.TrimSpace(in.ID) == "" || t.ID == strings.TrimSpace(in.ID)) {
				task = t
			}
		}
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"updated": true,
		"profile": profileView(p),
		"task":    task,
	}})
}

func hasArg(args map[string]any, key string) bool {
	_, ok := args[key]
	return ok
}

func taskFieldPresent(raw any, key string) bool {
	obj, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	_, ok = obj[key]
	return ok
}

// handleAgentImpact reports what depends on an agent — shown before retiring or
// removing so the operator sees the effects (M846).
func (s *Server) handleAgentImpact(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
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
func (s *Server) handleAgentTombstone(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	impact := s.agentImpactResult(p)
	manager := strings.TrimSpace(p.ParentAgent)
	if manager == "" {
		manager = strings.TrimSpace(p.OwnerAgent)
	}
	tombstone := map[string]any{
		"slug":             p.Slug,
		"name":             p.Name,
		"kind":             p.Kind(),
		"system":           p.System,
		"description":      p.Description,
		"manager":          manager,
		"retired":          p.Retired,
		"retired_ms":       p.RetiredMS,
		"retired_reason":   p.RetiredReason,
		"lifecycle_mode":   strings.TrimSpace(p.Lifecycle.Mode),
		"completed_cycles": p.Lifecycle.CompletedCycles,
		"max_cycles":       p.Lifecycle.MaxCycles,
		"memory_scope":     strings.TrimSpace(p.MemoryScope),
		"model":            strings.TrimSpace(p.Model),
		// Durable footprint left behind — the counts the removal cascade would act on.
		"footprint": map[string]any{
			"standing_orders":  impact["standing_count"],
			"schedules":        impact["schedule_count"],
			"memories":         impact["memory_count"],
			"authored_shared":  impact["authored_shared_memory_count"],
			"skills":           impact["skill_count"],
			"configs":          impact["config_count"],
			"workspaces":       impact["workspace_count"],
			"workflow_refs":    impact["workflow_ref_count"],
			"mailbox_messages": impact["mailbox_message_count"],
			"subagents":        impact["subagent_count"],
		},
		// Mailbox/audit messages and workflow refs are retained by design, not
		// deleted, so the tombstone records them as the agent's lasting trace.
		"retained_by_design": map[string]any{
			"mailbox_messages": impact["mailbox_message_count"],
			"workflow_refs":    impact["workflow_ref_count"],
		},
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"tombstone": tombstone}})
}

// handleAgentGraveyard lists retired agents with their retirement age — the
// read-only retention-eligibility view (NEXT.md #7). Optional older_than_days
// filters to long-dead identities. It REPORTS only; archiving / hard-removal stays
// an explicit operator action (no auto-deletion).
func (s *Server) handleAgentGraveyard(conn net.Conn, req Request) {
	var olderThanDays float64
	switch v := req.Args["older_than_days"].(type) {
	case float64:
		olderThanDays = v
	case string:
		olderThanDays, _ = strconv.ParseFloat(strings.TrimSpace(v), 64)
	}
	nowMS := time.Now().UnixMilli()
	cutoffMS := int64(0)
	if olderThanDays > 0 {
		cutoffMS = nowMS - int64(olderThanDays*24*3600*1000)
	}
	rows := make([]map[string]any, 0)
	for _, p := range s.k.Roster().List() {
		if !p.Retired {
			continue
		}
		if cutoffMS > 0 && p.RetiredMS > cutoffMS {
			continue // not yet older than the requested window
		}
		ageDays := 0.0
		if p.RetiredMS > 0 {
			ageDays = float64(nowMS-p.RetiredMS) / (24 * 3600 * 1000)
		}
		rows = append(rows, map[string]any{
			"slug":           p.Slug,
			"name":           p.Name,
			"kind":           p.Kind(),
			"system":         p.System,
			"retired_ms":     p.RetiredMS,
			"retired_reason": p.RetiredReason,
			"age_days":       int(ageDays),
		})
	}
	// Oldest first — the most retention-eligible identities lead.
	sort.SliceStable(rows, func(i, j int) bool {
		return plInt64(rows[i], "retired_ms") < plInt64(rows[j], "retired_ms")
	})
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"graveyard":       rows,
		"count":           len(rows),
		"older_than_days": int(olderThanDays),
	}})
}

func plInt64(m map[string]any, key string) int64 {
	switch n := m[key].(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func (s *Server) agentImpactResult(p roster.Profile) map[string]any {
	orders := s.k.AgentImpact(p.Slug)
	schedules := s.agentScheduleImpact(p.Slug)
	memories := s.agentMemoryImpact(p)
	authoredSharedMemories := s.agentAuthoredSharedMemoryImpact(p.Slug)
	skills := s.agentSkillImpact(p.Slug)
	configs := s.agentConfigImpact(p.Slug)
	workspaces := s.agentWorkspaceImpact(p)
	workflows := s.agentWorkflowImpact(p.Slug)
	mailboxMessages := s.agentMailboxImpact(p.Slug)
	children := s.agentSubagents(p.Slug)
	subagents := agentSubagentImpact(p.Slug, children)
	subagentOrders := s.agentSubagentStandingImpact(children)
	subagentSchedules := s.agentSubagentScheduleImpact(children)
	subagentMemories := s.agentSubagentMemoryImpact(children)
	subagentAuthoredSharedMemories := s.agentSubagentAuthoredSharedMemoryImpact(children)
	subagentSkills := s.agentSubagentSkillImpact(children)
	subagentConfigs := s.agentSubagentConfigImpact(children)
	subagentWorkspaces := s.agentSubagentWorkspaceImpact(children)
	subagentWorkflows := s.agentSubagentWorkflowImpact(children)
	subagentMailboxMessages := s.agentSubagentMailboxImpact(children)
	return map[string]any{
		"slug":            p.Slug,
		"standing_orders": orders, "standing_count": len(orders),
		"schedules": schedules, "schedule_count": len(schedules),
		"memories": memories, "memory_count": len(memories),
		"authored_shared_memories": authoredSharedMemories, "authored_shared_memory_count": len(authoredSharedMemories),
		"skills": skills, "skill_count": len(skills),
		"configs": configs, "config_count": len(configs),
		"workspaces": workspaces, "workspace_count": len(workspaces),
		"workflow_refs": workflows, "workflow_ref_count": len(workflows),
		"mailbox_messages": mailboxMessages, "mailbox_message_count": len(mailboxMessages),
		"subagents": subagents, "subagent_count": len(subagents),
		"subagent_standing_orders": subagentOrders, "subagent_standing_count": len(subagentOrders),
		"subagent_schedules": subagentSchedules, "subagent_schedule_count": len(subagentSchedules),
		"subagent_memories": subagentMemories, "subagent_memory_count": len(subagentMemories),
		"subagent_authored_shared_memories": subagentAuthoredSharedMemories, "subagent_authored_shared_memory_count": len(subagentAuthoredSharedMemories),
		"subagent_skills": subagentSkills, "subagent_skill_count": len(subagentSkills),
		"subagent_configs": subagentConfigs, "subagent_config_count": len(subagentConfigs),
		"subagent_workspaces": subagentWorkspaces, "subagent_workspace_count": len(subagentWorkspaces),
		"subagent_workflow_refs": subagentWorkflows, "subagent_workflow_ref_count": len(subagentWorkflows),
		"subagent_mailbox_messages": subagentMailboxMessages, "subagent_mailbox_message_count": len(subagentMailboxMessages),
	}
}

// handleAgentActivity builds a per-agent activity timeline (M854): what an agent
// did — the runs it executed, the council consults and sub-agent delegations
// during those runs, the memory it wrote (M851 actor), its board messages, and
// changes to its own profile. Derived entirely from the journal (no new store),
// newest first. Answers the owner's "what happened, which agent consulted for advice".
func (s *Server) handleAgentActivity(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	slug := p.Slug
	limit := 50
	if raw, ok := req.Args["limit"].(float64); ok && raw > 0 {
		limit = int(raw)
	}
	if limit > 500 {
		limit = 500
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
	if len(items) > limit {
		items = items[:limit]
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"slug": slug, "activity": items, "count": len(items), "total": total,
	}})
}

// handleAgentRepairStatus folds the journal into one agent's autonomous
// self-repair history: queued/completed/failed doctor.auto_repair events,
// newest first, plus the current inflight fingerprints and effective cooldown.
func (s *Server) handleAgentRepairStatus(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	limit := 20
	if raw, ok := req.Args["limit"].(float64); ok && raw > 0 {
		limit = int(raw)
	}
	if limit > 100 {
		limit = 100
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
	history := rows
	if len(history) > limit {
		history = history[:limit]
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

func (s *Server) handleAgentRepair(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	if p.Retired {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is retired — revive it first"})
		return
	}
	if !p.Enabled {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is paused"})
		return
	}
	if !p.AllowsDirectCall() {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: managedSubagentDirectCallError(p, "repaired")})
		return
	}
	corr := s.k.NewCorrelation()
	reason := strings.TrimSpace(stringArg(req.Args, "reason"))
	lineage := operatorIncidentLineage(req.Args)
	publishOperatorAction(s.k, "agent.repair", corr, map[string]any{
		"phase":              "requested",
		"agent":              p.Slug,
		"reason":             reason,
		"incident_id":        lineage.incidentID,
		"root_incident_id":   lineage.rootIncidentID,
		"parent_incident_id": lineage.parentIncidentID,
	})
	go func() {
		src := overseertool.NewKernelSource(s.k, s.baseDir)
		res, err := src.RepairAgent(p.Slug, reason)
		if err != nil {
			publishOperatorAction(s.k, "agent.repair", corr, map[string]any{
				"phase":              "failed",
				"agent":              p.Slug,
				"reason":             reason,
				"error":              err.Error(),
				"incident_id":        lineage.incidentID,
				"root_incident_id":   lineage.rootIncidentID,
				"parent_incident_id": lineage.parentIncidentID,
			})
			return
		}
		publishOperatorAction(s.k, "agent.repair", corr, map[string]any{
			"phase":                             "completed",
			"agent":                             p.Slug,
			"reason":                            reason,
			"applied":                           res.Applied,
			"routing_task_type":                 res.RoutingTaskType,
			"routing_task_model_chain":          res.RoutingTaskModelChain,
			"previous_routing_task_model_chain": res.PreviousRoutingTaskModelChain,
			"answer":                            truncate(res.Answer, 300),
			"incident_id":                       lineage.incidentID,
			"root_incident_id":                  lineage.rootIncidentID,
			"parent_incident_id":                lineage.parentIncidentID,
		})
	}()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"accepted":       true,
		"agent":          p.Slug,
		"correlation_id": corr,
	}})
}

func (s *Server) handleAgentWake(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	if p.Retired {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is retired — revive it first"})
		return
	}
	if !p.Enabled {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is paused"})
		return
	}
	if !p.AllowsDirectCall() {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: managedSubagentDirectCallError(p, "called")})
		return
	}
	intent, _, ierr := argString(req.Args, "intent")
	if ierr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: ierr.Error()})
		return
	}
	reason := strings.TrimSpace(stringArg(req.Args, "reason"))
	intent = buildOperatorWakeIntent(strings.TrimSpace(intent), p.Slug, reason, req.Args)
	if strings.TrimSpace(intent) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent wake requires args.intent or args.reason"})
		return
	}
	corr := s.k.NewCorrelation()
	lineage := operatorIncidentLineage(req.Args)
	runbook := agentAutonomyRunbookPayload(p)
	publishOperatorAction(s.k, "agent.wake", corr, map[string]any{
		"phase":              "requested",
		"agent":              p.Slug,
		"reason":             reason,
		"intent":             truncate(intent, 240),
		"autonomy_runbook":   runbook,
		"incident_id":        lineage.incidentID,
		"root_incident_id":   lineage.rootIncidentID,
		"parent_incident_id": lineage.parentIncidentID,
	})
	go s.runAgentWake(corr, p, intent, reason, lineage)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"accepted":       true,
		"agent":          p.Slug,
		"correlation_id": corr,
	}})
}

func (s *Server) handleAgentResolve(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	resolution := strings.TrimSpace(stringArg(req.Args, "resolution"))
	switch resolution {
	case "paused", "retired", "delegated", "force_chain":
	default:
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.resolution must be paused, retired, delegated, or force_chain"})
		return
	}
	summary := strings.TrimSpace(stringArg(req.Args, "summary"))
	lineage := operatorIncidentLineage(req.Args)
	corr := s.k.NewCorrelation()
	requested := map[string]any{
		"phase":              "requested",
		"agent":              p.Slug,
		"resolution":         resolution,
		"resolution_summary": summary,
		"incident_id":        lineage.incidentID,
		"root_incident_id":   lineage.rootIncidentID,
		"parent_incident_id": lineage.parentIncidentID,
	}
	if delegateTo := strings.TrimSpace(stringArg(req.Args, "delegate_to")); delegateTo != "" {
		requested["delegate_to"] = delegateTo
	}
	if taskType := strings.TrimSpace(stringArg(req.Args, "task_type")); taskType != "" {
		requested["routing_task_type"] = taskType
	}
	if chain, ok := req.Args["task_model_chain"].([]any); ok && len(chain) > 0 {
		requested["routing_task_model_chain"] = normalizeTaskModelChain(chain)
	}
	publishOperatorAction(s.k, "agent.resolve", corr, requested)

	result, err := s.applyAgentResolution(p, resolution, summary, req.Args)
	if err != nil {
		fail := map[string]any{
			"phase":              "failed",
			"agent":              p.Slug,
			"resolution":         resolution,
			"resolution_summary": summary,
			"reason":             err.Error(),
			"incident_id":        lineage.incidentID,
			"root_incident_id":   lineage.rootIncidentID,
			"parent_incident_id": lineage.parentIncidentID,
		}
		if result.delegateTo != "" {
			fail["delegate_to"] = result.delegateTo
		}
		if result.taskType != "" {
			fail["routing_task_type"] = result.taskType
		}
		if len(result.taskModelChain) > 0 {
			fail["routing_task_model_chain"] = result.taskModelChain
		}
		publishOperatorAction(s.k, "agent.resolve", corr, fail)
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	completed := map[string]any{
		"phase":              "completed",
		"agent":              p.Slug,
		"resolution":         resolution,
		"resolution_summary": summary,
		"incident_id":        lineage.incidentID,
		"root_incident_id":   lineage.rootIncidentID,
		"parent_incident_id": lineage.parentIncidentID,
	}
	if result.delegateTo != "" {
		completed["delegate_to"] = result.delegateTo
	}
	if result.messageID != "" {
		completed["message_id"] = result.messageID
	}
	if result.taskType != "" {
		completed["routing_task_type"] = result.taskType
	}
	if len(result.taskModelChain) > 0 {
		completed["routing_task_model_chain"] = result.taskModelChain
	}
	if len(result.previousTaskModelChain) > 0 {
		completed["previous_routing_task_model_chain"] = result.previousTaskModelChain
	}
	if result.routingForceGeneration > 0 {
		completed["routing_force_generation"] = result.routingForceGeneration
	}
	if result.previousRoutingForceGeneration > 0 {
		completed["previous_routing_force_generation"] = result.previousRoutingForceGeneration
	}
	publishOperatorAction(s.k, "agent.resolve", corr, completed)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"applied":        true,
		"agent":          p.Slug,
		"resolution":     resolution,
		"correlation_id": corr,
	}})
}

type appliedAgentResolution struct {
	delegateTo                     string
	messageID                      string
	taskType                       string
	taskModelChain                 []string
	previousTaskModelChain         []string
	routingForceGeneration         int
	previousRoutingForceGeneration int
}

type routingChainApplier interface {
	ApplyRoutingChain(ref, taskType string, targetChain []string, reason string) (overseertool.RepairResult, error)
}

func (s *Server) applyAgentResolution(p roster.Profile, resolution, summary string, args map[string]any) (appliedAgentResolution, error) {
	switch resolution {
	case "paused":
		if p.Retired {
			return appliedAgentResolution{}, fmt.Errorf("agent %s is retired — revive it first", p.Slug)
		}
		_, err := s.k.SetProfileEnabled(p.Slug, false)
		return appliedAgentResolution{}, err
	case "retired":
		reason := summary
		if reason == "" {
			reason = "retired by operator incident resolution"
		}
		_, err := s.k.SetProfileRetired(p.Slug, true, reason)
		return appliedAgentResolution{}, err
	case "delegated":
		target := strings.TrimSpace(stringArg(args, "delegate_to"))
		if err := s.validateOperatorDelegateTarget(p, target); err != nil {
			return appliedAgentResolution{}, err
		}
		st, ok := s.boardWriter()
		if !ok {
			return appliedAgentResolution{}, fmt.Errorf("the board is not available on this daemon")
		}
		text := strings.TrimSpace(summary)
		if text == "" {
			text = "Operator delegated this incident for ownership review."
		}
		msg, err := st.HelpRequest("operator", target, text, time.Now().UnixMilli())
		if err != nil {
			return appliedAgentResolution{}, err
		}
		if s.boardNotify != nil {
			s.boardNotify(msg, "")
		}
		return appliedAgentResolution{delegateTo: target, messageID: strings.TrimSpace(msg.ID)}, nil
	case "force_chain":
		taskType := strings.TrimSpace(stringArg(args, "task_type"))
		chain := normalizeTaskModelChain(argListAny(args["task_model_chain"]))
		if taskType == "" || len(chain) == 0 {
			return appliedAgentResolution{}, fmt.Errorf("force_chain resolution requires task_type and task_model_chain")
		}
		if exhausted := latestExhaustedRoutingChain(s.k, p.Slug, operatorIncidentLineage(args), taskType); len(exhausted) > 0 && equalStringSlices(exhausted, chain) {
			return appliedAgentResolution{}, fmt.Errorf("force_chain resolution must choose a new chain for exhausted routing policy")
		}
		src, ok := overseertool.NewKernelSource(s.k, s.baseDir).(routingChainApplier)
		if !ok {
			return appliedAgentResolution{}, fmt.Errorf("force_chain resolution is not supported by the active repair source")
		}
		prevGen := latestOperatorForceGeneration(s.k, p.Slug, taskType)
		res, err := src.ApplyRoutingChain(p.Slug, taskType, chain, summary)
		if err != nil {
			return appliedAgentResolution{}, err
		}
		return appliedAgentResolution{
			taskType:                       firstNonEmpty(res.RoutingTaskType, taskType),
			taskModelChain:                 append([]string(nil), firstNonEmptyStrings(res.RoutingTaskModelChain, chain)...),
			previousTaskModelChain:         append([]string(nil), res.PreviousRoutingTaskModelChain...),
			routingForceGeneration:         prevGen + 1,
			previousRoutingForceGeneration: prevGen,
		}, nil
	default:
		return appliedAgentResolution{}, nil
	}
}

func latestOperatorForceGeneration(k *runtime.Kernel, slug, taskType string) int {
	if k == nil || strings.TrimSpace(slug) == "" || strings.TrimSpace(taskType) == "" {
		return 0
	}
	best := 0
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindInfo || (e.Subject != "doctor.auto_repair" && e.Subject != "agent.resolve") {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		if strings.TrimSpace(plString(pl, "agent")) != slug || strings.TrimSpace(plString(pl, "resolution")) != "force_chain" {
			return nil
		}
		phase := strings.TrimSpace(plString(pl, "phase"))
		if phase != "resolution_applied" && phase != "completed" {
			return nil
		}
		if strings.TrimSpace(plString(pl, "routing_task_type")) != taskType {
			return nil
		}
		if gen := intNumber(pl["routing_force_generation"]); gen > best {
			best = gen
		}
		return nil
	})
	return best
}

func (s *Server) validateOperatorDelegateTarget(p roster.Profile, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return fmt.Errorf("delegated resolution requires delegate_to")
	}
	if strings.EqualFold(target, strings.TrimSpace(p.Slug)) {
		return fmt.Errorf("delegated resolution points back to the root agent %s", p.Slug)
	}
	if owner := firstNonEmpty(p.ParentAgent, p.OwnerAgent); owner != "" && strings.EqualFold(target, owner) {
		return fmt.Errorf("delegated resolution points back to the current owner %s", owner)
	}
	dst, ok := s.k.Roster().Get(target)
	if !ok {
		return fmt.Errorf("delegated resolution target %s does not exist", target)
	}
	if dst.Retired {
		return fmt.Errorf("delegated resolution target %s is retired", dst.Slug)
	}
	if !dst.AllowsDirectCall() {
		return fmt.Errorf("delegated resolution target %s is a managed sub-agent", dst.Slug)
	}
	return nil
}

func latestExhaustedRoutingChain(k *runtime.Kernel, slug string, lineage operatorWakeLineage, taskType string) []string {
	if k == nil || strings.TrimSpace(slug) == "" || strings.TrimSpace(taskType) == "" || !lineage.hasAny() {
		return nil
	}
	var bestChain []string
	var bestSeq int64
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindInfo || e.Subject != "doctor.auto_repair" {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		if !strings.EqualFold(strings.TrimSpace(plString(pl, "agent")), slug) {
			return nil
		}
		if strings.TrimSpace(plString(pl, "phase")) != "routing_force_exhausted_detected" {
			return nil
		}
		if !strings.EqualFold(strings.TrimSpace(plString(pl, "routing_task_type")), taskType) {
			return nil
		}
		if !incidentLineageMatchesPayload(lineage, pl) || e.Seq <= bestSeq {
			return nil
		}
		bestSeq = e.Seq
		bestChain = plStrings(pl, "routing_task_model_chain")
		return nil
	})
	return append([]string(nil), bestChain...)
}

func incidentLineageMatchesPayload(lineage operatorWakeLineage, pl map[string]any) bool {
	if !lineage.hasAny() {
		return false
	}
	payloadIDs := []string{
		strings.TrimSpace(plString(pl, "incident_id")),
		strings.TrimSpace(plString(pl, "root_incident_id")),
		strings.TrimSpace(plString(pl, "parent_incident_id")),
	}
	return incidentIDInSlice(lineage.incidentID, payloadIDs) ||
		incidentIDInSlice(lineage.rootIncidentID, payloadIDs) ||
		incidentIDInSlice(lineage.parentIncidentID, payloadIDs)
}

func incidentIDInSlice(id string, items []string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, item := range items {
		if strings.EqualFold(id, strings.TrimSpace(item)) {
			return true
		}
	}
	return false
}

func (l operatorWakeLineage) hasAny() bool {
	return strings.TrimSpace(l.incidentID) != "" ||
		strings.TrimSpace(l.rootIncidentID) != "" ||
		strings.TrimSpace(l.parentIncidentID) != ""
}

func argListAny(v any) []any {
	if raw, ok := v.([]any); ok {
		return raw
	}
	return nil
}

func normalizeTaskModelChain(raw []any) []string {
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		switch v := item.(type) {
		case string:
			if v = strings.TrimSpace(v); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(strings.TrimSpace(a[i]), strings.TrimSpace(b[i])) {
			return false
		}
	}
	return true
}

func (s *Server) handleAgentEscalations(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	limit := 20
	if raw, ok := req.Args["limit"].(float64); ok && raw > 0 {
		limit = int(raw)
	}
	if limit > 100 {
		limit = 100
	}
	st, err := s.boardReader()
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	rows := s.agentEscalationRows(st, p.Slug, limit)
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
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"slug":        p.Slug,
		"escalations": out,
		"count":       len(out),
		"open_count":  openCount,
	}})
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
func agentActivitySummary(e *event.Event, pl map[string]any, slug string, runCorr map[string]bool) (string, bool) {
	if e.Subject == "doctor.auto_repair" && e.Kind == event.KindInfo && plString(pl, "agent") == slug {
		mode := strings.TrimSpace(plString(pl, "mode"))
		phase := strings.TrimSpace(plString(pl, "phase"))
		forceGenSuffix := func() string {
			gen := intNumber(pl["routing_force_generation"])
			if gen > 1 {
				return " gen " + strconv.Itoa(gen)
			}
			return ""
		}
		prefix := "repair"
		if mode == "degraded" {
			prefix = "doctor"
		} else if mode == "routing" {
			prefix = "routing"
		}
		switch phase {
		case "routing_force_exhausted_detected":
			if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
				return "forced chain exhausted for " + truncate(taskType, 40) + forceGenSuffix(), true
			}
			return "forced chain exhausted" + forceGenSuffix(), true
		case "routing_forced_failed_detected":
			if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
				return "forced chain failed for " + truncate(taskType, 40) + forceGenSuffix(), true
			}
			return "forced chain failed after probation" + forceGenSuffix(), true
		case "routing_unstable_detected":
			if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
				return "routing instability detected for " + truncate(taskType, 40), true
			}
			return "routing instability detected", true
		case "attempts_exhausted":
			attempt := plInt(pl, "self_repair_attempt")
			maxAttempts := plInt(pl, "self_repair_max_attempts")
			if attempt > 0 && maxAttempts > 0 {
				return prefix + " attempts exhausted " + strconv.Itoa(attempt) + "/" + strconv.Itoa(maxAttempts), true
			}
			return prefix + " attempts exhausted", true
		case "queued":
			if mode == "degraded" {
				return prefix + " queued: " + truncate(plString(pl, "reason"), 80), true
			}
			if mode == "routing" {
				if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
					return "routing queued for " + truncate(taskType, 40), true
				}
				return prefix + " queued: " + truncate(plString(pl, "reason"), 80), true
			}
			issues := len(plStrings(pl, "issues"))
			if issues > 0 {
				return prefix + " queued for " + strconv.Itoa(issues) + " config issue(s)", true
			}
			return prefix + " queued: " + truncate(plString(pl, "reason"), 80), true
		case "routing_rollback_queued":
			if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
				return "routing rollback queued for " + truncate(taskType, 40), true
			}
			return "routing rollback queued", true
		case "completed":
			if mode == "routing" {
				if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
					if chain := strings.Join(plStrings(pl, "routing_task_model_chain"), " -> "); chain != "" {
						return "routing rewrote " + truncate(taskType, 30) + " to " + truncate(chain, 80), true
					}
					return "routing rewrote " + truncate(taskType, 30) + " chain", true
				}
			}
			applied := len(plStrings(pl, "applied"))
			if applied > 0 {
				return prefix + " applied " + strconv.Itoa(applied) + " profile change(s)", true
			}
			return prefix + " completed", true
		case "routing_rollback_completed":
			if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
				if chain := strings.Join(plStrings(pl, "routing_task_model_chain"), " -> "); chain != "" {
					return "routing rolled back " + truncate(taskType, 30) + " to " + truncate(chain, 80), true
				}
				return "routing rolled back " + truncate(taskType, 30) + " chain", true
			}
			return "routing rollback completed", true
		case "failed":
			if err := strings.TrimSpace(plString(pl, "error")); err != "" {
				return prefix + " failed: " + truncate(err, 80), true
			}
			return prefix + " failed", true
		case "routing_rollback_failed":
			if err := strings.TrimSpace(plString(pl, "error")); err != "" {
				return "routing rollback failed: " + truncate(err, 80), true
			}
			return "routing rollback failed", true
		case "resolution_failed":
			if res := strings.TrimSpace(plString(pl, "resolution")); res != "" {
				return "resolution " + res + " failed: " + truncate(plString(pl, "reason"), 80), true
			}
			return "resolution follow-up failed: " + truncate(plString(pl, "reason"), 80), true
		case "delegation_queued":
			if to := strings.TrimSpace(plString(pl, "delegate_to")); to != "" {
				return "manager delegated escalation to " + truncate(to, 60), true
			}
			return "delegation queued", true
		case "delegation_woke":
			if to := strings.TrimSpace(plString(pl, "delegate_to")); to != "" {
				return "delegated wake launched for " + truncate(to, 60), true
			}
			return "delegated wake launched", true
		case "delegation_failed":
			if to := strings.TrimSpace(plString(pl, "delegate_to")); to != "" {
				return "delegated wake failed for " + truncate(to, 60) + ": " + truncate(plString(pl, "reason"), 80), true
			}
			return "delegated wake failed: " + truncate(plString(pl, "reason"), 80), true
		case "escalation_answered":
			switch res := strings.TrimSpace(plString(pl, "resolution")); res {
			case "delegated":
				if to := strings.TrimSpace(plString(pl, "delegate_to")); to != "" {
					return "manager delegated escalation to " + truncate(to, 60), true
				}
				return "manager delegated escalation", true
			case "force_chain":
				if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
					if chain := strings.Join(plStrings(pl, "routing_task_model_chain"), " -> "); chain != "" {
						return "manager forced " + truncate(taskType, 30) + " to " + truncate(chain, 80) + forceGenSuffix(), true
					}
					return "manager forced " + truncate(taskType, 30) + " chain" + forceGenSuffix(), true
				}
				return "manager forced a routing chain" + forceGenSuffix(), true
			case "retired":
				return "manager retired the agent after escalation", true
			case "paused":
				return "manager paused the agent after escalation", true
			case "blocked":
				return "manager marked escalation blocked", true
			default:
				return "manager answered escalation", true
			}
		case "resolution_applied":
			switch res := strings.TrimSpace(plString(pl, "resolution")); res {
			case "force_chain":
				if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
					if chain := strings.Join(plStrings(pl, "routing_task_model_chain"), " -> "); chain != "" {
						return "manager applied forced " + truncate(taskType, 30) + " to " + truncate(chain, 80) + forceGenSuffix(), true
					}
					return "manager applied forced " + truncate(taskType, 30) + " chain" + forceGenSuffix(), true
				}
				return "manager applied a forced routing chain" + forceGenSuffix(), true
			case "retired":
				return "manager retirement was applied", true
			case "paused":
				return "manager pause was applied", true
			default:
				return "manager resolution applied", true
			}
		default:
			if phase != "" {
				return prefix + " " + phase, true
			}
		}
	}
	if e.Subject == "agent.repair" && e.Kind == event.KindInfo && plString(pl, "agent") == slug {
		switch phase := strings.TrimSpace(plString(pl, "phase")); phase {
		case "requested":
			return "operator requested a governed repair run", true
		case "completed":
			if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
				if chain := strings.Join(plStrings(pl, "routing_task_model_chain"), " -> "); chain != "" {
					return "operator rewrote " + truncate(taskType, 30) + " to " + truncate(chain, 80), true
				}
				return "operator rewrote " + truncate(taskType, 30) + " chain", true
			}
			if applied := len(plStrings(pl, "applied")); applied > 0 {
				return "operator repair applied " + strconv.Itoa(applied) + " profile change(s)", true
			}
			return "operator repair completed", true
		case "failed":
			return "operator repair failed: " + truncate(plString(pl, "error"), 80), true
		}
	}
	if e.Subject == "agent.wake" && e.Kind == event.KindInfo && plString(pl, "agent") == slug {
		contract := wakeRunbookActivitySuffix(pl)
		switch phase := strings.TrimSpace(plString(pl, "phase")); phase {
		case "requested":
			if reason := strings.TrimSpace(plString(pl, "reason")); reason != "" {
				return joinActivityParts("operator wake requested: "+truncate(reason, 80), contract), true
			}
			return joinActivityParts("operator wake requested", contract), true
		case "completed":
			return joinActivityParts("operator wake completed", contract), true
		case "failed":
			return joinActivityParts("operator wake failed: "+truncate(plString(pl, "error"), 80), contract), true
		}
	}
	if e.Kind == event.KindScheduleFired && plString(pl, "agent") == slug {
		contract := wakeRunbookActivitySuffix(pl)
		label := "schedule wake fired"
		if id := strings.TrimSpace(plString(pl, "schedule_id")); id != "" {
			label += ": " + truncate(id, 60)
		}
		return joinActivityParts(label, contract), true
	}
	if e.Kind == event.KindStandingFired && plString(pl, "agent") == slug {
		contract := wakeRunbookActivitySuffix(pl)
		// A board.* trigger subject is the mailbox-wake route: name the message
		// (and sender) that woke the agent rather than the standing order id.
		if isMailboxWakeSubject(plString(pl, "trigger_subject")) {
			tp, _ := pl["trigger_payload"].(map[string]any)
			label := "mailbox wake fired"
			if from := strings.TrimSpace(plString(tp, "from")); from != "" {
				label += ": from " + truncate(from, 60)
			} else if id := strings.TrimSpace(plString(tp, "id")); id != "" {
				label += ": " + truncate(id, 60)
			}
			return joinActivityParts(label, contract), true
		}
		label := "standing wake fired"
		// standing.fired addresses the order by "id"/"name" (not "standing_id").
		if id := strings.TrimSpace(firstNonEmpty(plString(pl, "standing_id"), plString(pl, "id"))); id != "" {
			label += ": " + truncate(id, 60)
		}
		return joinActivityParts(label, contract), true
	}
	if e.Kind == event.KindSubAgentSpawned && plString(pl, "agent") == slug {
		contract := wakeRunbookActivitySuffix(pl)
		label := "delegated wake fired"
		if by := strings.TrimSpace(plString(pl, "delegated_by")); by != "" {
			label += ": by " + truncate(by, 60)
		}
		return joinActivityParts(label, contract), true
	}
	if e.Subject == "agent.retire" && e.Kind == event.KindInfo && plString(pl, "agent") == slug {
		parts := []string{"operator retired the agent"}
		if reason := strings.TrimSpace(plString(pl, "reason")); reason != "" {
			parts = append(parts, "reason: "+truncate(reason, 80))
		}
		if paused := pausedTriggerSummary(pl); paused != "" {
			parts = append(parts, paused)
		}
		return strings.Join(parts, " · "), true
	}
	if e.Subject == "agent.revive" && e.Kind == event.KindInfo && plString(pl, "agent") == slug {
		if paused := pausedTriggerSummary(pl); paused != "" {
			return "operator revived the agent · " + paused, true
		}
		return "operator revived the agent", true
	}
	if e.Subject == "agent.remove" && e.Kind == event.KindInfo && plString(pl, "agent") == slug {
		if cleanup := removalCleanupSummary(pl); cleanup != "" {
			return "operator removed the agent · " + cleanup, true
		}
		return "operator removed the agent", true
	}
	if e.Subject == "agent.resolve" && e.Kind == event.KindInfo && plString(pl, "agent") == slug {
		switch phase := strings.TrimSpace(plString(pl, "phase")); phase {
		case "requested":
			if res := strings.TrimSpace(plString(pl, "resolution")); res != "" {
				return "operator requested resolution " + res, true
			}
			return "operator requested a resolution", true
		case "completed":
			switch res := strings.TrimSpace(plString(pl, "resolution")); res {
			case "force_chain":
				if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
					if chain := strings.Join(plStrings(pl, "routing_task_model_chain"), " -> "); chain != "" {
						return "operator forced " + truncate(taskType, 30) + " to " + truncate(chain, 80), true
					}
				}
				return "operator forced a routing chain", true
			case "delegated":
				if to := strings.TrimSpace(plString(pl, "delegate_to")); to != "" {
					return "operator delegated incident to " + truncate(to, 60), true
				}
				return "operator delegated incident", true
			case "paused":
				return "operator paused the agent", true
			case "retired":
				return "operator retired the agent", true
			default:
				return "operator resolution completed", true
			}
		case "failed":
			if res := strings.TrimSpace(plString(pl, "resolution")); res != "" {
				return "operator resolution " + res + " failed: " + truncate(plString(pl, "reason"), 80), true
			}
			return "operator resolution failed: " + truncate(plString(pl, "reason"), 80), true
		}
	}
	if e.Subject == "doctor.auto_repair" && e.Kind == event.KindInfo && plString(pl, "target_agent") == slug {
		switch phase := strings.TrimSpace(plString(pl, "phase")); phase {
		case "escalation_answered":
			switch res := strings.TrimSpace(plString(pl, "resolution")); res {
			case "delegated":
				if to := strings.TrimSpace(plString(pl, "delegate_to")); to != "" {
					return "delegated escalation for " + truncate(plString(pl, "agent"), 60) + " to " + truncate(to, 40), true
				}
				return "delegated escalation for " + truncate(plString(pl, "agent"), 60), true
			case "force_chain":
				if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
					return "forced routing chain for " + truncate(plString(pl, "agent"), 60) + " on " + truncate(taskType, 30), true
				}
				return "forced routing chain for " + truncate(plString(pl, "agent"), 60), true
			case "retired":
				return "retired " + truncate(plString(pl, "agent"), 60) + " after escalation", true
			case "paused":
				return "paused " + truncate(plString(pl, "agent"), 60) + " after escalation", true
			case "blocked":
				return "marked escalation blocked for " + truncate(plString(pl, "agent"), 60), true
			default:
				return "answered escalation for " + truncate(plString(pl, "agent"), 60), true
			}
		case "escalation_woke":
			return joinActivityParts("accepted escalation wake for "+truncate(plString(pl, "agent"), 60), wakeRunbookActivitySuffix(pl)), true
		case "escalation_skipped":
			return "skipped escalation wake: " + truncate(plString(pl, "reason"), 80), true
		case "escalation_failed":
			return "escalation wake failed: " + truncate(plString(pl, "reason"), 80), true
		case "delegation_queued":
			return "received delegated escalation for " + truncate(plString(pl, "agent"), 60), true
		case "delegation_woke":
			return joinActivityParts("accepted delegated escalation for "+truncate(plString(pl, "agent"), 60), wakeRunbookActivitySuffix(pl)), true
		case "delegation_failed":
			return "delegated escalation wake failed: " + truncate(plString(pl, "reason"), 80), true
		case "resolution_applied":
			switch res := strings.TrimSpace(plString(pl, "resolution")); res {
			case "force_chain":
				if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
					return "applied forced routing chain for " + truncate(plString(pl, "agent"), 60) + " on " + truncate(taskType, 30), true
				}
				return "applied forced routing chain for " + truncate(plString(pl, "agent"), 60), true
			case "retired":
				return "applied retirement for " + truncate(plString(pl, "agent"), 60), true
			case "paused":
				return "applied pause for " + truncate(plString(pl, "agent"), 60), true
			default:
				return "applied resolution for " + truncate(plString(pl, "agent"), 60), true
			}
		}
	}
	switch e.Kind {
	case event.KindTaskReceived:
		if plString(pl, "agent") == slug {
			return "started a run: " + truncate(plString(pl, "intent"), 100), true
		}
	case event.KindTaskCompleted:
		if runCorr[e.CorrelationID] {
			return "completed a run", true
		}
	case event.KindTaskFailed:
		if runCorr[e.CorrelationID] {
			r := plString(pl, "reason")
			if r == "" {
				r = "failed"
			}
			return "run failed: " + truncate(r, 80), true
		}
	case event.KindAgentRetry:
		if runCorr[e.CorrelationID] || plString(pl, "agent") == slug {
			attempt := plInt(pl, "next_attempt")
			maxAttempts := plInt(pl, "max_attempts")
			reason := firstNonEmpty(plString(pl, "reason"), plString(pl, "error"))
			policy := agentRetryPolicySummary(pl)
			if attempt > 0 && maxAttempts > 0 {
				return "retrying run: attempt " + strconv.Itoa(attempt) + "/" + strconv.Itoa(maxAttempts) + " after " + truncate(reason, 80) + policy, true
			}
			return "retrying run: " + truncate(reason, 80) + policy, true
		}
	case event.KindCouncilConvened:
		if runCorr[e.CorrelationID] {
			return "consulted the council: " + truncate(plString(pl, "question"), 100), true
		}
	case event.KindSubAgentSpawned:
		// The agent delegated (its run spawned a sub-agent), or it WAS the named
		// sub-agent that ran.
		if runCorr[e.CorrelationID] {
			return "delegated to a sub-agent: " + truncate(plString(pl, "agent"), 60), true
		}
		if plString(pl, "agent") == slug {
			return "ran as a delegated sub-agent", true
		}
	case event.KindMemoryWritten:
		if plString(pl, "actor") == slug {
			return "memory " + plString(pl, "action") + ": " + truncate(plString(pl, "subject"), 80), true
		}
	case event.KindBoardPosted:
		if plString(pl, "from") == slug {
			if to := plString(pl, "to"); to != "" {
				return "messaged " + to, true
			}
			return "posted to the board: " + truncate(plString(pl, "topic"), 60), true
		}
	case event.KindRosterUpdated:
		if plString(pl, "slug") == slug {
			a := plString(pl, "action")
			if a == "" {
				a = "updated"
			}
			if a == "lifecycle_cycle_completed" {
				completed := plInt(pl, "completed_cycles")
				max := plInt(pl, "max_cycles")
				if max > 0 {
					return "completed lifecycle cycle " + strconv.Itoa(completed) + "/" + strconv.Itoa(max), true
				}
				return "completed lifecycle cycle " + strconv.Itoa(completed), true
			}
			if a == "retired" {
				reason := strings.TrimSpace(plString(pl, "reason"))
				if strings.Contains(strings.ToLower(reason), "completed") {
					if reason != "" {
						return "lifecycle retired the agent: " + truncate(reason, 100), true
					}
					return "lifecycle retired the agent", true
				}
				if reason != "" {
					return "profile retired: " + truncate(reason, 100), true
				}
				return "profile retired", true
			}
			if a == "revived" {
				return "profile revived", true
			}
			return "profile " + a, true
		}
	}
	return "", false
}

func agentRetryPolicySummary(pl map[string]any) string {
	var bits []string
	if delay := plInt(pl, "delay_ms"); delay > 0 {
		bits = append(bits, "delay "+strconv.Itoa(delay)+"ms")
	}
	if backoff := strings.TrimSpace(plString(pl, "backoff")); backoff != "" {
		bits = append(bits, "backoff "+backoff)
	}
	if retryOn := plStrings(pl, "retry_on"); len(retryOn) > 0 {
		bits = append(bits, "retry_on "+strings.Join(retryOn, ","))
	}
	if len(bits) == 0 {
		return ""
	}
	return " (" + strings.Join(bits, "; ") + ")"
}

func pausedTriggerSummary(pl map[string]any) string {
	var bits []string
	if n := plInt(pl, "standing_paused"); n > 0 {
		bits = append(bits, strconv.Itoa(n)+" standing paused")
	}
	if n := plInt(pl, "schedules_paused"); n > 0 {
		bits = append(bits, strconv.Itoa(n)+" schedules paused")
	}
	return strings.Join(bits, ", ")
}

func removalCleanupSummary(pl map[string]any) string {
	var bits []string
	fields := []struct {
		key   string
		label string
	}{
		{"standing_removed", "standing removed"},
		{"schedules_removed", "schedules removed"},
		{"memories_forgotten", "private memories forgotten"},
		{"authored_memories_forgotten", "authored memories forgotten"},
		{"skills_archived", "skills archived"},
		{"configs_deleted", "configs deleted"},
		{"configs_access_pruned", "shared config access pruned"},
		{"workspaces_deleted", "workspaces deleted"},
		{"subagents_retired", "sub-agents retired"},
		{"mailbox_messages_retained", "mailbox/audit messages retained"},
		{"workflow_refs_retained", "workflow refs retained"},
		{"subagent_workflow_refs_retained", "sub-agent workflow refs retained"},
	}
	for _, f := range fields {
		if n := plInt(pl, f.key); n > 0 {
			bits = append(bits, strconv.Itoa(n)+" "+f.label)
		}
	}
	return strings.Join(bits, ", ")
}

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

func (s *Server) agentEscalationRows(st *board.Store, slug string, limit int) []agentEscalationRow {
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
	if len(out) > limit {
		out = out[:limit]
	}
	return out
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

func plString(pl map[string]any, key string) string {
	s, _ := pl[key].(string)
	return s
}

func plInt(pl map[string]any, key string) int {
	switch n := pl[key].(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func plStrings(pl map[string]any, key string) []string {
	raw, ok := pl[key].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			return item
		}
	}
	return ""
}

func firstNonEmptyStrings(primary, fallback []string) []string {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

func intNumber(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func (s *Server) handleAgentRetire(conn net.Conn, req Request) {
	s.handleAgentSetRetired(conn, req, true)
}

func (s *Server) handleAgentRevive(conn net.Conn, req Request) {
	s.handleAgentSetRetired(conn, req, false)
}

func (s *Server) handleAgentSetRetired(conn net.Conn, req Request, retired bool) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	reason := stringArg(req.Args, "reason")
	// Compute impact BEFORE the state change so a retire reports what it affected.
	var impact []string
	var impactSummary map[string]any
	if retired {
		if p, ok := s.k.Roster().Get(ref); ok {
			impact = s.k.AgentImpact(p.Slug)
			impactSummary = s.agentImpactResult(p)
		}
	} else if p, ok := s.k.Roster().Get(ref); ok {
		if err := s.validateAgentHierarchyRefs(p); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
			return
		}
	}
	p, err := s.k.SetProfileRetired(ref, retired, reason)
	if err != nil {
		if errors.Is(err, roster.ErrNotFound) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	res := map[string]any{"profile": profileView(p)}
	if retired {
		pausedStanding, err := s.pauseAgentStanding(p.Slug)
		if err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
			return
		}
		pausedSchedules, err := s.pauseAgentSchedules(p.Slug)
		if err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
			return
		}
		res["impact"] = impact
		res["impact_summary"] = impactSummary
		res["standing_paused"] = pausedStanding
		res["schedules_paused"] = pausedSchedules
		if impactSummary != nil {
			impactSummary["standing_paused"] = pausedStanding
			impactSummary["schedules_paused"] = pausedSchedules
		}
		publishOperatorAction(s.k, "agent.retire", s.k.NewCorrelation(), map[string]any{
			"agent":            p.Slug,
			"reason":           p.RetiredReason,
			"retired_ms":       p.RetiredMS,
			"standing_paused":  pausedStanding,
			"schedules_paused": pausedSchedules,
			"impact_summary":   impactSummary,
		})
	} else {
		pausedStanding := s.countAgentPausedStanding(p.Slug)
		pausedSchedules := s.countAgentPausedSchedules(p.Slug)
		res["standing_paused"] = pausedStanding
		res["schedules_paused"] = pausedSchedules
		publishOperatorAction(s.k, "agent.revive", s.k.NewCorrelation(), map[string]any{
			"agent":            p.Slug,
			"standing_paused":  pausedStanding,
			"schedules_paused": pausedSchedules,
		})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: res})
}

func (s *Server) handleAgentRemove(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	p, found := s.k.Roster().Get(ref)
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": false}})
		return
	}
	if p.System {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "system agent " + p.Slug + " cannot be removed; retire or pause it instead"})
		return
	}
	cascade := parseAgentRemoveCascade(req.Args["cascade"])
	subagents := s.agentSubagents(p.Slug)
	if len(subagents) > 0 && !cascade.Subagents {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: fmt.Sprintf("agent %s has %d dependent sub-agent(s); set cascade.subagents=true to retire them before removal", p.Slug, len(subagents))})
		return
	}
	retainedMailboxMessageLabels := s.agentRemovalMailboxImpact(p.Slug, subagents, cascade.Subagents)
	retainedWorkflowRefLabels := s.agentWorkflowImpact(p.Slug)
	retainedSubagentWorkflowRefLabels := []string(nil)
	if cascade.Subagents {
		retainedSubagentWorkflowRefLabels = s.agentSubagentWorkflowImpact(subagents)
	}
	retainedMailboxMessages := len(retainedMailboxMessageLabels)
	retainedWorkflowRefs := len(retainedWorkflowRefLabels)
	retainedSubagentWorkflowRefs := len(retainedSubagentWorkflowRefLabels)
	retiredSubagents, retiredSubagentSlugs, err := s.retireAgentSubagents(p.Slug, subagents, cascade.Subagents)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	removedStanding, err := s.removeAgentStanding(p.Slug, cascade.Standing)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	removedSchedules, err := s.removeAgentSchedules(p.Slug, cascade.Schedules)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if cascade.Subagents {
		for _, child := range subagents {
			n, err := s.removeAgentStanding(child.Slug, cascade.Standing)
			if err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
				return
			}
			removedStanding += n
			n, err = s.removeAgentSchedules(child.Slug, cascade.Schedules)
			if err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
				return
			}
			removedSchedules += n
		}
	}
	forgotMemory, err := s.forgetAgentMemory(p, cascade.Memory)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	forgotAuthoredMemory, err := s.forgetAgentAuthoredSharedMemory(p.Slug, cascade.AuthoredMemory)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	archivedSkills, err := s.archiveAgentSkills(p.Slug, cascade.Skills)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	deletedConfig, prunedConfigAccess, err := s.deleteAgentConfigEntries(p.Slug, cascade.Config)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	deletedWorkspaces, err := s.deleteAgentWorkspace(p, cascade.Workspace)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if cascade.Subagents {
		for _, child := range subagents {
			n, err := s.forgetAgentMemory(child, cascade.Memory)
			if err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
				return
			}
			forgotMemory += n
			n, err = s.forgetAgentAuthoredSharedMemory(child.Slug, cascade.AuthoredMemory)
			if err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
				return
			}
			forgotAuthoredMemory += n
			n, err = s.archiveAgentSkills(child.Slug, cascade.Skills)
			if err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
				return
			}
			archivedSkills += n
			var pruned int
			n, pruned, err = s.deleteAgentConfigEntries(child.Slug, cascade.Config)
			if err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
				return
			}
			deletedConfig += n
			prunedConfigAccess += pruned
			n, err = s.deleteAgentWorkspace(child, cascade.Workspace)
			if err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
				return
			}
			deletedWorkspaces += n
		}
	}
	ok, err := s.k.RemoveProfile(ref)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if ok {
		publishOperatorAction(s.k, "agent.remove", s.k.NewCorrelation(), map[string]any{
			"agent":                                  p.Slug,
			"removed":                                true,
			"cascade":                                agentRemoveCascadeView(cascade),
			"standing_removed":                       removedStanding,
			"schedules_removed":                      removedSchedules,
			"memories_forgotten":                     forgotMemory,
			"authored_memories_forgotten":            forgotAuthoredMemory,
			"skills_archived":                        archivedSkills,
			"configs_deleted":                        deletedConfig,
			"configs_access_pruned":                  prunedConfigAccess,
			"workspaces_deleted":                     deletedWorkspaces,
			"subagents_retired":                      retiredSubagents,
			"subagents_retired_slugs":                retiredSubagentSlugs,
			"mailbox_messages_retained":              retainedMailboxMessages,
			"mailbox_messages_retained_refs":         retainedMailboxMessageLabels,
			"workflow_refs_retained":                 retainedWorkflowRefs,
			"workflow_refs_retained_labels":          retainedWorkflowRefLabels,
			"subagent_workflow_refs_retained":        retainedSubagentWorkflowRefs,
			"subagent_workflow_refs_retained_labels": retainedSubagentWorkflowRefLabels,
		})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"removed":                                ok,
		"standing_removed":                       removedStanding,
		"schedules_removed":                      removedSchedules,
		"memories_forgotten":                     forgotMemory,
		"authored_memories_forgotten":            forgotAuthoredMemory,
		"skills_archived":                        archivedSkills,
		"configs_deleted":                        deletedConfig,
		"configs_access_pruned":                  prunedConfigAccess,
		"workspaces_deleted":                     deletedWorkspaces,
		"subagents_retired":                      retiredSubagents,
		"subagents_retired_slugs":                retiredSubagentSlugs,
		"mailbox_messages_retained":              retainedMailboxMessages,
		"mailbox_messages_retained_refs":         retainedMailboxMessageLabels,
		"workflow_refs_retained":                 retainedWorkflowRefs,
		"workflow_refs_retained_labels":          retainedWorkflowRefLabels,
		"subagent_workflow_refs_retained":        retainedSubagentWorkflowRefs,
		"subagent_workflow_refs_retained_labels": retainedSubagentWorkflowRefLabels,
	}})
}

type agentRemoveCascade struct {
	Standing       bool
	Schedules      bool
	Memory         bool
	AuthoredMemory bool
	Skills         bool
	Config         bool
	Workspace      bool
	Subagents      bool
}

func agentRemoveCascadeView(c agentRemoveCascade) map[string]any {
	return map[string]any{
		"standing":        c.Standing,
		"schedules":       c.Schedules,
		"memory":          c.Memory,
		"authored_memory": c.AuthoredMemory,
		"skills":          c.Skills,
		"config":          c.Config,
		"workspace":       c.Workspace,
		"subagents":       c.Subagents,
	}
}

func parseAgentRemoveCascade(raw any) agentRemoveCascade {
	var c agentRemoveCascade
	m, ok := raw.(map[string]any)
	if !ok {
		return c
	}
	c.Standing = boolish(m["standing"])
	c.Schedules = boolish(m["schedules"])
	c.Memory = boolish(m["memory"])
	c.AuthoredMemory = boolish(m["authored_memory"]) || boolish(m["authored_shared_memory"])
	c.Skills = boolish(m["skills"])
	c.Config = boolish(m["config"])
	c.Workspace = boolish(m["workspace"]) || boolish(m["workdir"])
	c.Subagents = boolish(m["subagents"])
	return c
}

func boolish(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		x = strings.TrimSpace(strings.ToLower(x))
		return x == "1" || x == "true" || x == "yes" || x == "on"
	default:
		return false
	}
}

func (s *Server) agentScheduleImpact(slug string) []string {
	var out []string
	for _, e := range s.k.Schedules().List() {
		if strings.EqualFold(strings.TrimSpace(e.Agent), slug) {
			label := e.Intent
			if strings.TrimSpace(label) == "" {
				label = e.ID
			}
			out = append(out, label+" ("+e.ID+")")
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentMemoryImpact(p roster.Profile) []string {
	scope := strings.TrimSpace(p.MemoryScope)
	if scope == "" {
		scope = p.Slug
	}
	records, err := s.k.Memory().Active()
	if err != nil {
		return nil
	}
	var out []string
	for _, r := range records {
		if memoryRecordBelongsToAgent(r, scope) {
			subj := strings.TrimSpace(r.Subject)
			if subj == "" {
				subj = r.ID
			}
			out = append(out, subj+" ("+r.ID+")")
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentAuthoredSharedMemoryImpact(slug string) []string {
	records, err := s.k.Memory().Active()
	if err != nil {
		return nil
	}
	var out []string
	for _, r := range records {
		if !memoryRecordAuthoredSharedByAgent(r, slug) {
			continue
		}
		subj := strings.TrimSpace(r.Subject)
		if subj == "" {
			subj = r.ID
		}
		out = append(out, subj+" ("+r.ID+")")
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentSkillImpact(slug string) []string {
	all, err := s.k.Forge().List()
	if err != nil {
		return nil
	}
	var out []string
	for _, sk := range all {
		if strings.EqualFold(strings.TrimSpace(sk.Agent), slug) && sk.Status != skill.StatusArchived {
			name := strings.TrimSpace(sk.Name)
			if name == "" {
				name = sk.ID
			}
			out = append(out, name+" ("+sk.ID+")")
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentConfigImpact(slug string) []string {
	if s.k.ConfigCenter() == nil {
		return nil
	}
	var out []string
	for _, e := range s.k.ConfigCenter().ListEntries() {
		if configEntryBelongsToAgent(e, slug) {
			label := strings.TrimSpace(e.Key)
			if e.Rating != "" {
				label += " [" + string(e.Rating) + "]"
			}
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentWorkspaceImpact(p roster.Profile) []string {
	info, ok := s.agentWorkspaceInfo(p)
	if !ok {
		return nil
	}
	return []string{info}
}

func (s *Server) agentWorkflowImpact(slug string) []string {
	slug = strings.TrimSpace(slug)
	if slug == "" || s.k.Workflows() == nil {
		return nil
	}
	var out []string
	for _, w := range s.k.Workflows().List() {
		for _, n := range w.Nodes {
			if !workflowNodeConfigReferencesAgent(n.Config, slug) {
				continue
			}
			label := w.Name + "/" + n.ID
			if strings.TrimSpace(n.Label) != "" {
				label += " " + strings.TrimSpace(n.Label)
			}
			label += " [" + n.Type + "]"
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentMailboxImpact(slug string) []string {
	st, err := s.boardReader()
	if err != nil {
		return nil
	}
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" {
		return nil
	}
	msgs := st.Read("", boardReadMaxLimit)
	out := make([]string, 0, 8)
	for _, msg := range msgs {
		label, ok := agentMailboxImpactLabel(msg, slug)
		if ok {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentRemovalMailboxImpact(slug string, subagents []roster.Profile, includeSubagents bool) []string {
	seen := map[string]bool{}
	add := func(labels []string) {
		for _, label := range labels {
			if strings.TrimSpace(label) != "" {
				seen[label] = true
			}
		}
	}
	add(s.agentMailboxImpact(slug))
	if includeSubagents {
		for _, child := range subagents {
			add(s.agentMailboxImpact(child.Slug))
		}
	}
	out := make([]string, 0, len(seen))
	for label := range seen {
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentSubagentMailboxImpact(children []roster.Profile) []string {
	out := make([]string, 0, len(children))
	for _, child := range children {
		for _, label := range s.agentMailboxImpact(child.Slug) {
			out = append(out, child.Slug+": "+label)
		}
	}
	sort.Strings(out)
	return out
}

func agentMailboxImpactLabel(msg board.Message, slug string) (string, bool) {
	from := strings.ToLower(strings.TrimSpace(msg.From))
	to := strings.ToLower(strings.TrimSpace(msg.To))
	acked := boardMessageAckedBy(msg, slug)
	var direction string
	switch {
	case from == slug:
		direction = "sent"
	case to == slug:
		direction = "received"
	case msg.To == board.Everyone && from != slug:
		direction = "broadcast"
	case acked:
		direction = "acked"
	default:
		return "", false
	}
	topic := strings.TrimSpace(msg.Topic)
	if topic == "" {
		topic = "board"
	}
	id := strings.TrimSpace(msg.ID)
	if id == "" {
		id = strconv.FormatInt(msg.TSMS, 10)
	}
	return topic + " " + direction + " (" + id + ")", true
}

func agentSubagentImpact(slug string, children []roster.Profile) []string {
	out := make([]string, 0, len(children))
	for _, child := range children {
		roles := make([]string, 0, 2)
		if strings.EqualFold(strings.TrimSpace(child.OwnerAgent), slug) {
			roles = append(roles, "owner")
		}
		if strings.EqualFold(strings.TrimSpace(child.ParentAgent), slug) {
			roles = append(roles, "parent")
		}
		if len(roles) == 0 {
			roles = append(roles, "descendant")
		}
		label := child.Slug
		if strings.TrimSpace(child.Name) != "" && strings.TrimSpace(child.Name) != child.Slug {
			label = strings.TrimSpace(child.Name) + " (" + child.Slug + ")"
		}
		if len(roles) > 0 {
			label += " [" + strings.Join(roles, ", ") + "]"
		}
		if child.Retired {
			label += " [retired]"
		}
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentSubagentStandingImpact(children []roster.Profile) []string {
	var out []string
	for _, child := range children {
		for _, label := range s.k.AgentImpact(child.Slug) {
			out = append(out, child.Slug+": "+label)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentSubagentScheduleImpact(children []roster.Profile) []string {
	var out []string
	for _, child := range children {
		for _, label := range s.agentScheduleImpact(child.Slug) {
			out = append(out, child.Slug+": "+label)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentSubagentMemoryImpact(children []roster.Profile) []string {
	var out []string
	for _, child := range children {
		for _, label := range s.agentMemoryImpact(child) {
			out = append(out, child.Slug+": "+label)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentSubagentAuthoredSharedMemoryImpact(children []roster.Profile) []string {
	var out []string
	for _, child := range children {
		for _, label := range s.agentAuthoredSharedMemoryImpact(child.Slug) {
			out = append(out, child.Slug+": "+label)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentSubagentSkillImpact(children []roster.Profile) []string {
	var out []string
	for _, child := range children {
		for _, label := range s.agentSkillImpact(child.Slug) {
			out = append(out, child.Slug+": "+label)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentSubagentConfigImpact(children []roster.Profile) []string {
	var out []string
	for _, child := range children {
		for _, label := range s.agentConfigImpact(child.Slug) {
			out = append(out, child.Slug+": "+label)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentSubagentWorkspaceImpact(children []roster.Profile) []string {
	var out []string
	for _, child := range children {
		for _, label := range s.agentWorkspaceImpact(child) {
			out = append(out, child.Slug+": "+label)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentSubagentWorkflowImpact(children []roster.Profile) []string {
	var out []string
	for _, child := range children {
		for _, label := range s.agentWorkflowImpact(child.Slug) {
			out = append(out, child.Slug+": "+label)
		}
	}
	sort.Strings(out)
	return out
}

func workflowNodeConfigReferencesAgent(raw json.RawMessage, slug string) bool {
	if len(raw) == 0 {
		return false
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	return jsonValueReferencesAgent(v, strings.ToLower(strings.TrimSpace(slug)), "")
}

func jsonValueReferencesAgent(v any, slug, key string) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			if jsonValueReferencesAgent(value, slug, strings.ToLower(strings.TrimSpace(k))) {
				return true
			}
		}
	case []any:
		for _, value := range x {
			if jsonValueReferencesAgent(value, slug, key) {
				return true
			}
		}
	case string:
		if !agentReferenceConfigKey(key) {
			return false
		}
		return strings.EqualFold(strings.TrimSpace(x), slug)
	}
	return false
}

func agentReferenceConfigKey(key string) bool {
	switch key {
	case "agent", "agent_slug", "target_agent", "owner_agent", "parent_agent", "delegate_to", "source_agent", "root_agent":
		return true
	default:
		return false
	}
}

func (s *Server) agentSubagents(slug string) []roster.Profile {
	root := strings.TrimSpace(slug)
	if root == "" {
		return nil
	}
	byManager := map[string][]roster.Profile{}
	for _, p := range s.k.Roster().List() {
		childSlug := strings.TrimSpace(p.Slug)
		if childSlug == "" || strings.EqualFold(childSlug, root) {
			continue
		}
		seenManager := map[string]bool{}
		for _, manager := range []string{strings.TrimSpace(p.OwnerAgent), strings.TrimSpace(p.ParentAgent)} {
			if manager == "" || strings.EqualFold(manager, childSlug) || seenManager[strings.ToLower(manager)] {
				continue
			}
			seenManager[strings.ToLower(manager)] = true
			byManager[strings.ToLower(manager)] = append(byManager[strings.ToLower(manager)], p)
		}
	}
	var out []roster.Profile
	seen := map[string]bool{strings.ToLower(root): true}
	var walk func(string)
	walk = func(parent string) {
		for _, child := range byManager[strings.ToLower(strings.TrimSpace(parent))] {
			key := strings.ToLower(strings.TrimSpace(child.Slug))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, child)
			walk(child.Slug)
		}
	}
	walk(root)
	sort.Slice(out, func(i, j int) bool {
		return strings.Compare(out[i].Slug, out[j].Slug) < 0
	})
	return out
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

func (s *Server) retireAgentSubagents(parent string, children []roster.Profile, on bool) (int, []string, error) {
	if !on {
		return 0, nil, nil
	}
	retired := 0
	slugs := []string{}
	reason := "parent/owner " + parent + " removed"
	for _, child := range children {
		if child.Retired {
			continue
		}
		if _, err := s.k.SetProfileRetired(child.Slug, true, reason); err != nil {
			return retired, slugs, err
		}
		if _, err := s.pauseAgentStanding(child.Slug); err != nil {
			return retired, slugs, err
		}
		if _, err := s.pauseAgentSchedules(child.Slug); err != nil {
			return retired, slugs, err
		}
		slugs = append(slugs, child.Slug)
		retired++
	}
	sort.Strings(slugs)
	return retired, slugs, nil
}

func (s *Server) removeAgentStanding(slug string, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	removed := 0
	for _, o := range s.k.Standing().List() {
		if strings.EqualFold(strings.TrimSpace(o.Agent), slug) {
			ok, err := s.k.RemoveStanding(o.ID)
			if err != nil {
				return removed, err
			}
			if ok {
				removed++
			}
		}
	}
	return removed, nil
}

func (s *Server) pauseAgentStanding(slug string) (int, error) {
	paused := 0
	for _, o := range s.k.Standing().List() {
		if !o.Enabled || !strings.EqualFold(strings.TrimSpace(o.Agent), slug) {
			continue
		}
		if _, err := s.k.SetStandingEnabled(o.ID, false); err != nil {
			return paused, err
		}
		paused++
	}
	return paused, nil
}

func (s *Server) countAgentPausedStanding(slug string) int {
	n := 0
	for _, o := range s.k.Standing().List() {
		if !o.Enabled && strings.EqualFold(strings.TrimSpace(o.Agent), slug) {
			n++
		}
	}
	return n
}

func (s *Server) removeAgentSchedules(slug string, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	removed := 0
	for _, e := range s.k.Schedules().List() {
		if strings.EqualFold(strings.TrimSpace(e.Agent), slug) {
			ok, err := s.k.Schedules().Remove(e.ID)
			if err != nil {
				return removed, err
			}
			if ok {
				removed++
			}
		}
	}
	return removed, nil
}

func (s *Server) pauseAgentSchedules(slug string) (int, error) {
	paused := 0
	for _, e := range s.k.Schedules().List() {
		if !e.Enabled || !strings.EqualFold(strings.TrimSpace(e.Agent), slug) {
			continue
		}
		ok, err := s.k.Schedules().SetEnabled(e.ID, false)
		if err != nil {
			return paused, err
		}
		if ok {
			paused++
		}
	}
	return paused, nil
}

func (s *Server) countAgentPausedSchedules(slug string) int {
	n := 0
	for _, e := range s.k.Schedules().List() {
		if e.Enabled || !strings.EqualFold(strings.TrimSpace(e.Agent), slug) {
			continue
		}
		n++
	}
	return n
}

func (s *Server) forgetAgentMemory(p roster.Profile, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	scope := strings.TrimSpace(p.MemoryScope)
	if scope == "" {
		scope = p.Slug
	}
	records, err := s.k.Memory().Active()
	if err != nil {
		return 0, err
	}
	forgot := 0
	for _, r := range records {
		if !memoryRecordBelongsToAgent(r, scope) {
			continue
		}
		ok, err := s.k.Memory().Forget("", r.ID)
		if err != nil {
			return forgot, err
		}
		if ok {
			forgot++
		}
	}
	return forgot, nil
}

func memoryRecordBelongsToAgent(r memory.Record, scope string) bool {
	if r.Tags != nil && strings.EqualFold(strings.TrimSpace(r.Tags["scope"]), scope) {
		return true
	}
	return false
}

func (s *Server) forgetAgentAuthoredSharedMemory(slug string, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	records, err := s.k.Memory().Active()
	if err != nil {
		return 0, err
	}
	forgot := 0
	for _, r := range records {
		if !memoryRecordAuthoredSharedByAgent(r, slug) {
			continue
		}
		ok, err := s.k.Memory().Forget("", r.ID)
		if err != nil {
			return forgot, err
		}
		if ok {
			forgot++
		}
	}
	return forgot, nil
}

func memoryRecordAuthoredSharedByAgent(r memory.Record, slug string) bool {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return false
	}
	if r.Tags != nil && strings.TrimSpace(r.Tags["scope"]) != "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.AddedBy), slug) || strings.EqualFold(strings.TrimSpace(r.UpdatedBy), slug)
}

func (s *Server) archiveAgentSkills(slug string, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	all, err := s.k.Forge().List()
	if err != nil {
		return 0, err
	}
	archived := 0
	for _, sk := range all {
		if !strings.EqualFold(strings.TrimSpace(sk.Agent), slug) || sk.Status == skill.StatusArchived {
			continue
		}
		if err := s.k.Forge().Archive("", sk.ID, "agent removed: "+slug); err != nil {
			if errors.Is(err, skill.ErrIllegalTransition) {
				continue
			}
			return archived, err
		}
		archived++
	}
	return archived, nil
}

func (s *Server) deleteAgentConfigEntries(slug string, on bool) (int, int, error) {
	if !on || s.k.ConfigCenter() == nil {
		return 0, 0, nil
	}
	var keys []string
	deleting := map[string]bool{}
	for _, e := range s.k.ConfigCenter().ListEntries() {
		if configEntryBelongsToAgent(e, slug) {
			keys = append(keys, e.Key)
			deleting[e.Key] = true
		}
	}
	sort.Strings(keys)
	deleted := 0
	for _, key := range keys {
		if err := s.k.ConfigCenter().Delete(key); err != nil {
			return deleted, 0, err
		}
		deleted++
	}
	pruned := 0
	for _, e := range s.k.ConfigCenter().ListEntries() {
		if e == nil || deleting[e.Key] || !pruneConfigEntryAgentAccess(e, slug) {
			continue
		}
		if err := s.k.ConfigCenter().Set(e); err != nil {
			return deleted, pruned, err
		}
		pruned++
	}
	return deleted, pruned, nil
}

func (s *Server) deleteAgentWorkspace(p roster.Profile, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	workdir := strings.TrimSpace(p.Workdir)
	if workdir == "" {
		return 0, nil
	}
	root := s.agentWorkspaceRoot()
	dir, ok := confineUnder(root, workdir)
	if !ok || filepath.Clean(dir) == filepath.Clean(root) {
		return 0, fmt.Errorf("agent %s workspace path is unsafe: %s", p.Slug, workdir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("agent %s workspace is not a directory: %s", p.Slug, workdir)
	}
	if err := os.RemoveAll(dir); err != nil {
		return 0, err
	}
	return 1, nil
}

func pruneConfigEntryAgentAccess(e *configcenter.ConfigEntry, slug string) bool {
	if e == nil {
		return false
	}
	nextAllowed, allowedChanged := removeAgentAccessRef(e.AllowedAgents, slug)
	nextExcluded, excludedChanged := removeAgentAccessRef(e.ExcludedAgents, slug)
	if allowedChanged {
		e.AllowedAgents = nextAllowed
	}
	if excludedChanged {
		e.ExcludedAgents = nextExcluded
	}
	return allowedChanged || excludedChanged
}

func removeAgentAccessRef(values []string, slug string) ([]string, bool) {
	slug = strings.TrimSpace(slug)
	if slug == "" || len(values) == 0 {
		return values, false
	}
	out := values[:0]
	changed := false
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), slug) {
			changed = true
			continue
		}
		out = append(out, value)
	}
	if !changed {
		return values, false
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, true
}

func configEntryBelongsToAgent(e *configcenter.ConfigEntry, slug string) bool {
	if e == nil {
		return false
	}
	slug = strings.TrimSpace(strings.ToLower(slug))
	if slug == "" {
		return false
	}
	key := strings.TrimSpace(strings.ToLower(e.Key))
	for _, prefix := range []string{
		"agent/" + slug + "/",
		"agents/" + slug + "/",
		"agent." + slug + ".",
		"agents." + slug + ".",
	} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	if strings.EqualFold(strings.TrimSpace(e.CreatedBy), slug) {
		return true
	}
	for _, tag := range e.Tags {
		t := strings.TrimSpace(strings.ToLower(tag))
		if t == "agent:"+slug || t == "agent/"+slug || t == "owner:"+slug || t == "owner/"+slug {
			return true
		}
	}
	for _, k := range []string{"agent", "agent_slug", "owner_agent", "parent_agent"} {
		if strings.EqualFold(strings.TrimSpace(e.Metadata[k]), slug) {
			return true
		}
	}
	return false
}
