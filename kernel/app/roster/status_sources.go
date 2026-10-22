// SPDX-License-Identifier: MIT

package roster

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/event"
	core "github.com/agezt/agezt/kernel/modules/agents/roster"
	"github.com/agezt/agezt/kernel/modules/board/board"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
)

// RunRow is one folded run row, as the status collection needs it. The host
// folds its journal into these rows; the application owns what a row MEANS
// (which runs count as active, how a live status reads).
type RunRow struct {
	CorrelationID     string
	Intent            string
	Model             string
	Agent             string
	SpentMicrocents   int64
	StartedUnixMS     int64
	ParentCorrelation string
	// The terminal markers. A row with none of them is still running.
	Completed bool
	Failed    bool
	Abandoned bool
}

// Status reports the row's status. Precedence: completed > failed >
// abandoned > running — so a row that somehow carries several terminal
// markers reports the most authoritative one.
func (r RunRow) Status() string {
	switch {
	case r.Completed:
		return "completed"
	case r.Failed:
		return "failed"
	case r.Abandoned:
		return "abandoned"
	default:
		return "running"
	}
}

// StatusSources are the host's data sources for the roster status.
type StatusSources struct {
	// Journal walks the durable journal once. The collection dispatches every
	// event into every accumulator in that single walk.
	Journal func(func(*event.Event) error) error
	// Runs returns the folded run rows.
	Runs func() ([]RunRow, error)
	// Schedules lists the cadence entries that can wake an agent.
	Schedules func() []cadence.Entry
	// Standing lists the standing orders that can wake an agent.
	Standing func() []standing.Order
	// Board lists the board's open help requests — the escalations.
	Board func() []board.Message
	// Reaper runs the reaper scan over the window.
	Reaper func(reaperCut int64) runtime.ReaperReport
	// RepairCooldown is how long a repair stays ineligible. Nil reads the
	// environment.
	RepairCooldown func() time.Duration
}

// statusAccums holds the per-agent state that the journal-derivable helpers
// accumulate. Collecting every accumulator in a SINGLE journal walk turns what
// used to be O(journalSize) per helper (and 11x that across all helpers) into
// one O(journalSize) walk. Large, busy journals were reliably tripping the
// control-plane connection's read deadline under the previous 11-Range design
// (each Range does a callback-driven O(n) walk over every durable event, with
// JSON-unmarshal + map lookups per event). The single-pass dispatch below is
// behavior-preserving: each accumulator's key-set and "latest wins" semantics
// match the original per-helper implementations.
//
// Trade-offs: liveStatuses requires a precomputed run map; for the roster path
// we keep the small lookup inside the same walk by threading runAgent through
// the closure. Repair summaries and wake/escalation views use external
// stores, so they remain separate helpers and don't participate in this
// single pass.
type statusAccums struct {
	liveStatuses     map[string]LiveStatus
	lastActivities   map[string]LastActivity
	autonomyRunbooks map[string]map[string]any
	mailboxWakes     map[string]map[string]any
	policyDenials    map[string]PolicyDenials
	routingCounts    map[string]RoutingPressure
	retryCounts      map[string]RetryPressure
	// The doctor's own repair events: the latest per agent, and the latest
	// per fingerprint (an in-flight repair is one fingerprint's row).
	repairRows          map[string]RepairRow
	repairByFingerprint map[string]map[string]RepairRow
	routingCut          int64
}

// CollectJournalStatus fills the snapshot's journal-derived fields: the live
// run statuses, the last activity per agent, the last autonomy runbook, the
// mailbox wakes, the policy denials, the routing/retry pressure and the
// repair summaries.
//
// The journal is walked ONCE, dispatching every event into every relevant
// per-agent accumulator. Callers reading the snapshot later see the same
// final maps the previous 11-Range implementation produced; only the number of
// journal walks is reduced (11 -> 1), and the doctor's repair events now
// share that walk instead of taking one of their own.
func CollectJournalStatus(src StatusSources, profiles []core.Profile, routingCut int64, snap *StatusSnapshot) {
	acc := fillStatusAccums(src, profiles, routingCut)
	snap.Live = acc.liveStatuses
	snap.LastActivities = acc.lastActivities
	snap.Runbooks = acc.autonomyRunbooks
	snap.MailboxWakes = acc.mailboxWakes
	snap.PolicyDenials = acc.policyDenials
	snap.RoutingCounts = acc.routingCounts
	snap.RetryCounts = acc.retryCounts
	snap.Repairs = repairSummaries(acc)
}

// fillStatusAccums walks the journal ONCE, dispatching every event into every
// relevant per-agent accumulator.
func fillStatusAccums(src StatusSources, profiles []core.Profile, routingCut int64) statusAccums {
	var acc statusAccums
	acc.routingCut = routingCut
	acc.liveStatuses = make(map[string]LiveStatus)
	acc.lastActivities = make(map[string]LastActivity, len(profiles))
	acc.autonomyRunbooks = map[string]map[string]any{}
	acc.mailboxWakes = map[string]map[string]any{}
	acc.policyDenials = make(map[string]PolicyDenials, len(profiles))
	acc.routingCounts = make(map[string]RoutingPressure, len(profiles))
	acc.retryCounts = make(map[string]RetryPressure, len(profiles))
	acc.repairRows = map[string]RepairRow{}
	acc.repairByFingerprint = map[string]map[string]RepairRow{}
	cooldown := repairCooldown(src)

	known := make(map[string]bool, len(profiles))
	for _, p := range profiles {
		known[p.Slug] = true
	}

	// Latest per-correlation -> slug for agent.retry attribution (since
	// agent.retry payloads don't always carry an agent slug).
	runAgent := map[string]string{}

	// LiveStatus: only consider currently-running rows so the runAgent map
	// stays small and the correlation lookup is fast.
	var rows []RunRow
	if src.Runs != nil {
		if folded, err := src.Runs(); err == nil {
			rows = folded
		}
	}
	runAgent4Live := map[string]string{}
	for _, r := range rows {
		if r.Status() != "running" || strings.TrimSpace(r.Agent) == "" || !known[r.Agent] {
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
	// If no runs are active the liveStatus inner Range is a no-op anyway, but
	// skipping it entirely saves the entire walk on the common idle-roster
	// case (matters most when most of the agent list is idle).
	hasActiveRuns := len(runAgent4Live) > 0

	if src.Journal == nil {
		return acc
	}
	_ = src.Journal(func(e *event.Event) error {
		// repairRows — the doctor's own repair events, the latest per agent
		// and the latest per fingerprint.
		if e.Subject == "doctor.auto_repair" && e.Kind == event.KindInfo {
			var pl map[string]any
			if json.Unmarshal(e.Payload, &pl) == nil {
				if slug := plString(pl, "agent"); strings.TrimSpace(slug) != "" {
					row := repairRow(e, pl, cooldown)
					if cur, ok := acc.repairRows[slug]; !ok || row.Seq > cur.Seq {
						acc.repairRows[slug] = row
					}
					if row.Fingerprint != "" {
						if acc.repairByFingerprint[slug] == nil {
							acc.repairByFingerprint[slug] = map[string]RepairRow{}
						}
						if cur, ok := acc.repairByFingerprint[slug][row.Fingerprint]; !ok || row.Seq > cur.Seq {
							acc.repairByFingerprint[slug][row.Fingerprint] = row
						}
					}
				}
			}
		}

		// lastActivities: full pass, mirror the activity summary contract.
		{
			var pl map[string]any
			_ = json.Unmarshal(e.Payload, &pl)
			for slug := range known {
				summary, ok := ActivitySummary(e, pl, slug, nil)
				if !ok {
					continue
				}
				cur := acc.lastActivities[slug]
				if e.TSUnixMS >= cur.TSUnixMS {
					acc.lastActivities[slug] = LastActivity{
						TSUnixMS:      e.TSUnixMS,
						Kind:          string(e.Kind),
						CorrelationID: e.CorrelationID,
						Summary:       summary,
					}
				}
			}
		}

		// autonomyRunbooks — the wake the agent last received.
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
							if IsMailboxWakeSubject(subj) {
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

		// mailboxWakes — the messages the agent was woken for.
		if e.Kind == event.KindStandingFired {
			var pl map[string]any
			if json.Unmarshal(e.Payload, &pl) == nil {
				slug := plString(pl, "agent")
				if known[slug] && IsMailboxWakeSubject(plString(pl, "trigger_subject")) {
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

		// policyDenials — the tool calls the policy refused. task.received
		// also feeds the retry runAgent below.
		if e.Kind == event.KindTaskReceived {
			var pl map[string]any
			if json.Unmarshal(e.Payload, &pl) == nil {
				if slug := plString(pl, "agent"); slug != "" && known[slug] && e.CorrelationID != "" {
					runAgent[e.CorrelationID] = slug
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

		// routingCounts — the model's chain fallbacks.
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
					if !routingMatchesProfile(p, pl.TaskType, pl.FailedModel, pl.NextModel) {
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

		// retryCounts — the agent's own retries. task.received is handled
		// above; this branch only fires on agent.retry.
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

		return nil
	})

	return acc
}

// applyActiveWakeContext folds the wake that started a live run into its row.
func applyActiveWakeContext(row LiveStatus, kind event.Kind, pl map[string]any) LiveStatus {
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

// liveEventSummary reads a live run's phase from the event that advanced it.
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

// routingMatchesProfile reports whether a model-chain fallback belongs to the
// profile: by its task type, or by one of the models in its chain.
func routingMatchesProfile(p core.Profile, taskType, failedModel, nextModel string) bool {
	taskType = strings.TrimSpace(taskType)
	failedModel = strings.TrimSpace(failedModel)
	nextModel = strings.TrimSpace(nextModel)
	if pt := strings.TrimSpace(p.TaskType); pt != "" && strings.EqualFold(pt, taskType) {
		return true
	}
	for _, model := range AgentModelChain(strings.TrimSpace(p.Model), p.Fallbacks) {
		if strings.EqualFold(model, failedModel) || strings.EqualFold(model, nextModel) {
			return true
		}
	}
	return false
}

// AgentModelChain puts the primary model in front and appends the profile's
// fallbacks, dropping blanks and duplicates.
func AgentModelChain(primary string, fallbacks []string) []string {
	chain := []string{primary}
	for _, m := range fallbacks {
		if m = strings.TrimSpace(m); m != "" && m != primary {
			chain = append(chain, m)
		}
	}
	return chain
}
