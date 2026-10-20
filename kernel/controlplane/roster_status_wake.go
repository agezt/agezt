// SPDX-License-Identifier: MIT

package controlplane

// Provenance: Wake state helpers: agentWakeStatusViews, scheduleEntryMatchesAgent,
//             legacyScheduleAgentSlug, scheduleWakeLabel. Code extracted from
//             roster_status.go during the Day-39 god-file split. Public API
//             unchanged. The live wake-context and event-summary helpers moved
//             to the application layer with the journal status collection.

import (
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/standing"
)

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
