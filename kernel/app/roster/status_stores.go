// SPDX-License-Identifier: MIT

package roster

import (
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/event"
	core "github.com/agezt/agezt/kernel/modules/agents/roster"
	"github.com/agezt/agezt/kernel/modules/board/board"
	"github.com/agezt/agezt/kernel/standing"
)

// CollectStatus assembles the whole roster status snapshot from the host's
// sources: the journal (the live run statuses, the last activity, the last
// autonomy runbook, the mailbox wakes, the policy denials, the routing and
// retry pressure AND the repair summaries), the schedules and standing orders
// (the wake views), the board (the escalations) and the reaper scan (the
// health picture).
//
// The journal is walked ONCE for everything the journal carries.
func CollectStatus(src StatusSources, profiles []core.Profile, reaperCut, routingCut int64) StatusSnapshot {
	snap := StatusSnapshot{
		Degraded: map[string]DegradedStatus{}, Misconfigured: map[string]MisconfigurationStatus{}, Routing: map[string]RoutingStatus{},
		Forced: map[string]ForcedStatus{}, ForcedFailed: map[string]ForcedStatus{}, ForcedExhausted: map[string]ForcedStatus{},
		Unstable: map[string]UnstableStatus{}, Dead: map[string]DeadStatus{},
	}
	CollectJournalStatus(src, profiles, routingCut, &snap)
	snap.Escalations = CollectEscalationStatus(src, profiles)
	snap.Wakes = CollectWakeStatus(src, profiles)
	collectReaperStatus(src, reaperCut, &snap)
	return snap
}

// CollectWakeStatus reads the schedules and the standing orders and reports,
// per agent, how many wake it, when it next wakes and which event subjects it
// listens to. An agent with neither a schedule nor a standing order has no
// wake status at all.
func CollectWakeStatus(src StatusSources, profiles []core.Profile) map[string]WakeStatus {
	if len(profiles) == 0 {
		return nil
	}
	out := make(map[string]WakeStatus, len(profiles))
	for _, p := range profiles {
		out[p.Slug] = WakeStatus{}
	}
	if src.Schedules != nil {
		for _, e := range src.Schedules() {
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
	}
	if src.Standing != nil {
		for _, o := range src.Standing() {
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
	}
	for slug, row := range out {
		if row.ScheduleCount == 0 && row.StandingCount == 0 {
			delete(out, slug)
		}
	}
	return out
}

// CollectEscalationStatus reads the board's open help requests and reports,
// per agent, how many are open and how many the agent already acknowledged.
func CollectEscalationStatus(src StatusSources, profiles []core.Profile) map[string]EscalationLoad {
	if len(profiles) == 0 {
		return nil
	}
	if src.Board == nil {
		return nil
	}
	messages := src.Board()
	if len(messages) == 0 {
		return nil
	}
	known := make(map[string]bool, len(profiles))
	out := make(map[string]EscalationLoad, len(profiles))
	for _, p := range profiles {
		known[strings.ToLower(strings.TrimSpace(p.Slug))] = true
	}
	for _, msg := range messages {
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
				if BoardMessageAckedBy(msg, slug) {
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
		if BoardMessageAckedBy(msg, to) {
			row.Acked++
		} else {
			row.Open++
		}
		out[to] = row
	}
	return out
}

// collectReaperStatus reads the reaper scan and reports the health picture:
// the degraded, misconfigured, forced, unstable and dead agents.
func collectReaperStatus(src StatusSources, reaperCut int64, snap *StatusSnapshot) {
	if src.Reaper == nil {
		return
	}
	rep := src.Reaper(reaperCut)
	for _, r := range rep.DegradedAgents {
		snap.Degraded[r.Slug] = DegradedStatus{Failures: r.Failures, Threshold: r.Threshold, Window: r.Window, LastFailureMS: r.LastFailureMS}
	}
	for _, r := range rep.MisconfiguredAgents {
		snap.Misconfigured[r.Slug] = MisconfigurationStatus{Issues: r.Issues}
	}
	for _, r := range rep.RoutingPressure {
		snap.Routing[r.Slug] = RoutingStatus{Count: r.Count}
	}
	for _, r := range rep.RoutingForced {
		snap.Forced[r.Slug] = ForcedStatus{Count: r.Count, TaskType: r.TaskType, ForcedChain: r.ForcedChain, ForceGeneration: r.ForceGeneration}
	}
	for _, r := range rep.RoutingForcedFailed {
		snap.ForcedFailed[r.Slug] = ForcedStatus{Count: r.Count, TaskType: r.TaskType, ForcedChain: r.ForcedChain, ForceGeneration: r.ForceGeneration}
	}
	for _, r := range rep.RoutingForcedExhausted {
		snap.ForcedExhausted[r.Slug] = ForcedStatus{Count: r.Count, TaskType: r.TaskType, ForcedChain: r.ForcedChain, ForceGeneration: r.ForceGeneration}
	}
	for _, r := range rep.RoutingUnstable {
		snap.Unstable[r.Slug] = UnstableStatus{Count: r.Count, TaskType: r.TaskType, CurrentChain: r.CurrentChain, PreviousChain: r.PreviousChain}
	}
	for _, r := range rep.DeadAgents {
		snap.Dead[r.Slug] = DeadStatus{LastActiveMS: r.LastActiveMS}
	}
}

// repairSummaries folds the doctor's repair events into one summary per
// agent: the latest repair, and how many distinct repairs are still in flight.
func repairSummaries(acc statusAccums) map[string]RepairSummary {
	out := map[string]RepairSummary{}
	for slug, latest := range acc.repairRows {
		sum := RepairSummary{Latest: latest, HasLatest: true}
		for _, row := range acc.repairByFingerprint[slug] {
			if row.Phase == "queued" || row.Phase == "routing_rollback_queued" {
				sum.InflightCount++
			}
		}
		out[slug] = sum
	}
	return out
}

// repairRow reads one doctor repair event.
func repairRow(e *event.Event, pl map[string]any, cooldown time.Duration) RepairRow {
	return RepairRow{
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
}

// AutoRepairCooldown is how long a repair stays ineligible after it ran. The
// host may override it; the default reads the environment. A malformed or
// non-positive value falls back to 30 minutes.
func AutoRepairCooldown() time.Duration {
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

// repairCooldown resolves the cooldown the host configured.
func repairCooldown(src StatusSources) time.Duration {
	if src.RepairCooldown != nil {
		if d := src.RepairCooldown(); d > 0 {
			return d
		}
	}
	return AutoRepairCooldown()
}

// scheduleEntryMatchesAgent reports whether a schedule wakes the agent: by
// its agent field, or by the legacy --agent flag in its intent.
func scheduleEntryMatchesAgent(e cadence.Entry, slug string) bool {
	if strings.EqualFold(strings.TrimSpace(e.Agent), slug) {
		return true
	}
	return strings.EqualFold(legacyScheduleAgentSlug(e.Intent), slug)
}

// legacyScheduleAgentSlug reads the agent a schedule's intent names with the
// legacy --agent flag.
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

// scheduleWakeLabel names what a schedule wakes: the workflow, the system
// task or the tool it drives, else its intent, else its id.
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

// uniqueStrings drops the blanks and the consecutive duplicates.
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
