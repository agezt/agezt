// SPDX-License-Identifier: MIT

// Per-agent status view: agentStatusViews renders fillAgentStatusAccumsFromJournal results.
// Code extracted from roster_status.go during the Day-39 god-file split. Public API unchanged.
package controlplane


import (
	"time"

	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
)


func (s *Server) agentStatusViews(profiles []roster.Profile) map[string]map[string]any {
	const reaperWindow = 30 * 24 * time.Hour
	const routingWindow = 24 * time.Hour
	cut := time.Now().Add(-reaperWindow).UnixMilli()
	routingCut := time.Now().Add(-routingWindow).UnixMilli()
	rep := s.k.ReaperScan(cut, cut)
	repairs := s.agentRepairSummaries()
	escalationLoads := s.agentEscalationLoadViews(profiles)
	wakeStatuses := s.agentWakeStatusViews(profiles)

	// Single-pass journal dispatch (11 → 1) — see agentStatusAccums comment.
	acc := s.fillAgentStatusAccumsFromJournal(profiles, routingCut)
	routingCounts := acc.routingCounts
	retryCounts := acc.retryCounts
	liveStatuses := acc.liveStatuses
	lastActivities := acc.lastActivities
	autonomyRunbooks := acc.autonomyRunbooks
	mailboxWakes := acc.mailboxWakes
	policyDenials := acc.policyDenials

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