// SPDX-License-Identifier: MIT

// Wake state helpers + agentWakeStatusViews: isMailboxWakeSubject, agentPolicyDenials, applyActiveWakeContext, liveEventSummary, scheduleEntryMatchesAgent, legacyScheduleAgentSlug, scheduleWakeLabel.
// Code extracted from roster_status.go during the Day-39 god-file split. Public API unchanged.
package controlplane


import (
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/standing"
)


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
