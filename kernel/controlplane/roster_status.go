// SPDX-License-Identifier: MIT

// Per-agent live status: the journal projection behind the roster's status
// column. One pass over the journal fills every accumulator (fill...Accums),
// which the view builders then render per agent — routing pressure, retry
// pressure, escalation load, wake state, and what the agent is doing right now.
// Split from roster.go (refactor Phase 3.5).

package controlplane

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
)

type agentStatusAccums struct {
	liveStatuses     map[string]agentLiveStatus
	lastActivities   map[string]agentLastActivity
	autonomyRunbooks map[string]map[string]any
	mailboxWakes     map[string]map[string]any
	policyDenials    map[string]agentPolicyDenials
	routingCounts    map[string]agentRoutingPressure
	retryCounts      map[string]agentRetryPressure
	routingCut       int64
}

// fillAgentStatusAccumsFromJournal walks the journal ONCE, dispatching every
// event into every relevant per-agent accumulator. Callers reading accums
// later see the same final maps the previous 11-Range implementation
// produced; only the number of journal walks is reduced (11 → 1).
//
// Trade-offs: liveStatuses requires a precomputed runAgent map; for the
// roster path we keep the small lookup inside the same Range by threading
// runAgent through the closure. Repair summaries
// (agentRepairSummaries) and wake/escalation views use external stores, so
// they remain separate helpers and don't participate in this single pass.
func (s *Server) fillAgentStatusAccumsFromJournal(profiles []roster.Profile, routingCut int64) agentStatusAccums {
	var acc agentStatusAccums
	acc.routingCut = routingCut
	acc.liveStatuses = make(map[string]agentLiveStatus)
	acc.lastActivities = make(map[string]agentLastActivity, len(profiles))
	acc.autonomyRunbooks = map[string]map[string]any{}
	acc.mailboxWakes = map[string]map[string]any{}
	acc.policyDenials = map[string]agentPolicyDenials{}
	acc.routingCounts = make(map[string]agentRoutingPressure, len(profiles))
	acc.retryCounts = make(map[string]agentRetryPressure, len(profiles))
	cooldown := agentAutoRepairCooldown()

	if len(profiles) == 0 {
		return acc
	}

	known := make(map[string]bool, len(profiles))
	for _, p := range profiles {
		known[p.Slug] = true
	}

	// Latest per-correlation → slug for agent.retry attribution (since
	// agent.retry payloads don't always carry an agent slug). Same
	// semantics as the retired agentRetryPressureViews helper.
	runAgent := map[string]string{}

	// LiveStatus: only consider currently-running rows so the runAgent
	// map stays small and `e.CorrelationID == runAgent[cid]` is fast.
	var runs map[string]*runEntry
	if rs, err := s.collectRuns(s.k); err == nil {
		runs = rs
	}
	runAgent4Live := map[string]string{}
	for _, r := range runs {
		if r == nil || runEntryStatus(r) != "running" || strings.TrimSpace(r.Agent) == "" || !known[r.Agent] {
			continue
		}
		runAgent4Live[r.CorrelationID] = r.Agent
		row := acc.liveStatuses[r.Agent]
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
		acc.liveStatuses[r.Agent] = row
	}
	// If no runs are active the liveStatus inner Range is a no-op anyway,
	// but skipping it entirely saves the entire walk on the common
	// idle-roster case (matters most when most of the agent list is idle).
	hasActiveRuns := len(runAgent4Live) > 0

	_ = s.k.Journal().Range(func(e *event.Event) error {
		// lastActivities: full pass, mirror agentActivitySummary contract.
		{
			var pl map[string]any
			_ = json.Unmarshal(e.Payload, &pl)
			for slug := range known {
				summary, ok := agentActivitySummary(e, pl, slug, nil)
				if !ok {
					continue
				}
				cur := acc.lastActivities[slug]
				if e.TSUnixMS >= cur.TSUnixMS {
					acc.lastActivities[slug] = agentLastActivity{
						TSUnixMS:      e.TSUnixMS,
						Kind:          string(e.Kind),
						CorrelationID: e.CorrelationID,
						Summary:       summary,
					}
				}
			}
		}

		// autonomyRunbooks — same key/subject conditions as the retired
		// agentLastAutonomyRunbookViews helper.
		if (e.Subject == "agent.wake" && e.Kind == event.KindInfo) || e.Kind == event.KindScheduleFired || e.Kind == event.KindStandingFired || e.Kind == event.KindSubAgentSpawned || (e.Subject == "doctor.auto_repair" && e.Kind == event.KindInfo) {
			var pl map[string]any
			if json.Unmarshal(e.Payload, &pl) == nil {
				isDoctorWake := e.Subject == "doctor.auto_repair" && e.Kind == event.KindInfo
				slug := plString(pl, "agent")
				if isDoctorWake {
					slug = plString(pl, "target_agent")
				}
				if known[slug] {
					if raw, ok := pl["autonomy_runbook"].(map[string]any); ok && len(raw) > 0 {
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
							cp["source"] = "standing"
							cp["standing_id"] = firstNonEmpty(plString(pl, "standing_id"), plString(pl, "id"))
							if name := firstNonEmpty(plString(pl, "standing_name"), plString(pl, "name")); name != "" {
								cp["standing_name"] = name
							}
							subj := plString(pl, "trigger_subject")
							if subj != "" {
								cp["trigger_subject"] = subj
							}
							if isMailboxWakeSubject(subj) {
								cp["wake_via"] = "mailbox"
								if tp, _ := pl["trigger_payload"].(map[string]any); tp != nil {
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
							}
						case e.Kind == event.KindSubAgentSpawned:
							cp["source"] = "delegated"
							if by := plString(pl, "delegated_by"); by != "" {
								cp["delegated_by"] = by
							}
							if pc := firstNonEmpty(plString(pl, "parent_correlation_id"), plString(pl, "parent")); pc != "" {
								cp["parent_correlation_id"] = pc
							}
						case isDoctorWake:
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
						acc.autonomyRunbooks[slug] = cp
					}
				}
			}
		}

		// mailboxWakes — same condition as the retired agentMailboxWakeViews helper.
		if e.Kind == event.KindStandingFired {
			var pl map[string]any
			if json.Unmarshal(e.Payload, &pl) == nil {
				slug := plString(pl, "agent")
				if known[slug] && isMailboxWakeSubject(plString(pl, "trigger_subject")) {
					if tp, _ := pl["trigger_payload"].(map[string]any); tp != nil {
						msgID := plString(tp, "id")
						if msgID != "" {
							byMsg := acc.mailboxWakes[slug]
							if byMsg == nil {
								byMsg = map[string]any{}
								acc.mailboxWakes[slug] = byMsg
							}
							byMsg[msgID] = map[string]any{
								"correlation_id":  e.CorrelationID,
								"ts_unix_ms":      e.TSUnixMS,
								"trigger_subject": plString(pl, "trigger_subject"),
							}
						}
					}
				}
			}
		}

		// policyDenials — mirrored from the retired agentPolicyDenialViews
		// helper. task.received
		// also feeds the retry runAgent below.
		if e.Kind == event.KindTaskReceived {
			var pl map[string]any
			if json.Unmarshal(e.Payload, &pl) == nil {
				if slug := plString(pl, "agent"); slug != "" && known[slug] && e.CorrelationID != "" {
					runAgent[e.CorrelationID] = slug
					// also seed lastActivity runCorr equivalent via task.received
				}
			}
		} else if e.Kind == event.KindPolicyDecision {
			var pl map[string]any
			if json.Unmarshal(e.Payload, &pl) == nil {
				if allow, ok := pl["allow"].(bool); !ok || allow {
					// pass
				} else if slug := runAgent[e.CorrelationID]; slug != "" {
					d := acc.policyDenials[slug]
					d.Count++
					d.LastTool = plString(pl, "tool")
					d.LastReason = plString(pl, "reason")
					d.LastCapability = plString(pl, "capability")
					d.LastHard, _ = pl["hard_denied"].(bool)
					d.LastTSMS = e.TSUnixMS
					acc.policyDenials[slug] = d
				}
			}
		}

		// routingCounts — mirrored from the retired agentRoutingPressureViews helper.
		if e.Kind == event.KindProviderFallback && e.TSUnixMS >= routingCut {
			var pl struct {
				FailedModel string `json:"failed_model"`
				NextModel   string `json:"next_model"`
				Reason      string `json:"reason"`
				Scope       string `json:"scope"`
				TaskType    string `json:"task_type"`
			}
			if json.Unmarshal(e.Payload, &pl) == nil && strings.TrimSpace(pl.Scope) == "model-chain" {
				for _, p := range profiles {
					if !agentRoutingMatchesProfile(p, pl.TaskType, pl.FailedModel, pl.NextModel) {
						continue
					}
					row := acc.routingCounts[p.Slug]
					row.Count++
					if e.TSUnixMS >= row.LastTSMS {
						row.LastReason = strings.TrimSpace(pl.Reason)
						row.LastFailed = strings.TrimSpace(pl.FailedModel)
						row.LastNext = strings.TrimSpace(pl.NextModel)
						row.LastTSMS = e.TSUnixMS
					}
					acc.routingCounts[p.Slug] = row
				}
			}
		}

		// retryCounts — mirrored from the retired agentRetryPressureViews
		// helper. task.received
		// is handled above; this branch only fires on agent.retry.
		if e.Kind == event.KindAgentRetry {
			var pl map[string]any
			_ = json.Unmarshal(e.Payload, &pl)
			slug := plString(pl, "agent")
			if slug == "" {
				slug = runAgent[e.CorrelationID]
			}
			if known[slug] {
				row := acc.retryCounts[slug]
				row.Count++
				if e.TSUnixMS >= row.LastTSMS {
					row.LastReason = firstNonEmpty(plString(pl, "reason"), plString(pl, "error"))
					row.LastTSMS = e.TSUnixMS
					row.NextAttempt = plInt(pl, "next_attempt")
					row.MaxAttempts = plInt(pl, "max_attempts")
				}
				acc.retryCounts[slug] = row
			}
		}

		// liveStatuses wake-context propagation — only if we have active runs.
		if hasActiveRuns {
			if agentSlug, ok := runAgent4Live[e.CorrelationID]; ok {
				row := acc.liveStatuses[agentSlug]
				if row.ActiveCorrelationID != e.CorrelationID {
					// skip — but we still need to return; restructure below.
					return nil
				}
				var pl map[string]any
				_ = json.Unmarshal(e.Payload, &pl)
				row = applyActiveWakeContext(row, e.Kind, pl)
				if e.TSUnixMS < row.ActiveLastEventMS {
					acc.liveStatuses[agentSlug] = row
					return nil
				}
				phase, detail, tool, iter := liveEventSummary(e.Kind, pl)
				if phase == "" {
					acc.liveStatuses[agentSlug] = row
					return nil
				}
				row.ActivePhase = phase
				row.ActiveDetail = detail
				row.ActiveTool = tool
				row.ActiveIter = iter
				row.ActiveLastEventMS = e.TSUnixMS
				row.ActiveLastEventKind = string(e.Kind)
				acc.liveStatuses[agentSlug] = row
			}
		}

		// cooldown is referenced by the repair-summary helper; keep it
		// captured at function scope to silence the unused-var lint path.
		_ = cooldown

		return nil
	})

	return acc
}

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
func isMailboxWakeSubject(subject string) bool {
	return subject == "board" || strings.HasPrefix(subject, "board.")
}

type agentPolicyDenials struct {
	Count          int
	LastTool       string
	LastReason     string
	LastCapability string
	LastHard       bool
	LastTSMS       int64
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
