// SPDX-License-Identifier: MIT

// Agent live status: types (agentStatusAccums) + fillAgentStatusAccumsFromJournal (single-pass fold).
// Code extracted from roster_status.go during the Day-39 god-file split. Public API unchanged.
package controlplane


import (
	"encoding/json"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
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
