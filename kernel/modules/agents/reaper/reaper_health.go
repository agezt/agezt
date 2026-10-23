// SPDX-License-Identifier: MIT

package reaper

// Reaper health-classification helpers (degraded + misconfigured +
// binding/hierarchy config issues).

import (
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/modules/agents/api"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
)

// degradedAgents finds the agents whose recent terminal runs crossed their
// profile's health threshold.
func degradedAgents(profiles []roster.Profile, runs map[string]agentRunHealth) []api.DegradedAgent {
	byAgent := map[string][]agentRunHealth{}
	for _, r := range runs {
		if r.agent == "" || r.status == "" {
			continue
		}
		byAgent[r.agent] = append(byAgent[r.agent], r)
	}
	var out []api.DegradedAgent
	for _, p := range profiles {
		if !p.Enabled || p.Retired || p.HealthPolicy == nil {
			continue
		}
		threshold := p.HealthPolicy.FailureThreshold
		if threshold <= 0 {
			continue
		}
		window := p.HealthPolicy.FailureWindow
		if window <= 0 {
			window = threshold
		}
		rows := byAgent[p.Slug]
		sort.Slice(rows, func(i, j int) bool { return rows[i].startedMS > rows[j].startedMS })
		if len(rows) > window {
			rows = rows[:window]
		}
		failures := 0
		var lastFailure agentRunHealth
		for _, r := range rows {
			if r.status == "failed" {
				failures++
				if r.endedMS > lastFailure.endedMS {
					lastFailure = r
				}
			}
		}
		if failures < threshold {
			continue
		}
		row := api.DegradedAgent{
			Slug:          p.Slug,
			Name:          p.Name,
			Failures:      failures,
			Window:        window,
			Threshold:     threshold,
			DoctorAgent:   p.HealthPolicy.DoctorAgent,
			LastFailureMS: lastFailure.endedMS,
			LastReason:    lastFailure.reason,
		}
		if p.SelfRepairPolicy != nil {
			row.SelfRepairEnabled = p.SelfRepairPolicy.Enabled
			row.EscalateTo = p.SelfRepairPolicy.EscalateTo
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// misconfiguredAgents finds the agents whose runtime overrides or hierarchy
// references are invalid, so one or more intended autonomy knobs will not
// actually apply until repaired.
func (s *Scanner) misconfiguredAgents(profiles []roster.Profile) []api.MisconfiguredAgent {
	var out []api.MisconfiguredAgent
	bySlug := make(map[string]roster.Profile, len(profiles))
	for _, p := range profiles {
		bySlug[p.Slug] = p
	}
	bindingIssues := s.bindingConfigIssues(bySlug)
	seen := map[string]bool{}
	for _, p := range profiles {
		bindings := bindingIssues[p.Slug]
		if len(bindings) == 0 && (!p.Enabled || p.Retired) {
			continue
		}
		issues := s.runtimeConfigIssues(p.ConfigOverrides)
		issues = append(issues, hierarchyConfigIssues(p, bySlug)...)
		issues = append(issues, bindings...)
		if len(issues) == 0 {
			continue
		}
		seen[p.Slug] = true
		row := api.MisconfiguredAgent{
			Slug: p.Slug,
			Name: p.Name,
		}
		for _, issue := range issues {
			row.Issues = append(row.Issues, issue.Key+": "+issue.Issue)
		}
		if p.HealthPolicy != nil {
			row.DoctorAgent = p.HealthPolicy.DoctorAgent
		}
		if p.SelfRepairPolicy != nil {
			row.SelfRepairEnabled = p.SelfRepairPolicy.Enabled
			row.EscalateTo = p.SelfRepairPolicy.EscalateTo
		}
		out = append(out, row)
	}
	for slug, issues := range bindingIssues {
		if seen[slug] || len(issues) == 0 {
			continue
		}
		row := api.MisconfiguredAgent{Slug: slug}
		for _, issue := range issues {
			row.Issues = append(row.Issues, issue.Key+": "+issue.Issue)
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// runtimeConfigIssues asks the host which of a profile's runtime overrides do
// not parse. The host owns the table of knobs it consumes, so the module and
// the doctor can never disagree about what is valid.
func (s *Scanner) runtimeConfigIssues(overrides map[string]string) []api.ConfigIssue {
	if s.ports.RuntimeConfigIssues == nil {
		return nil
	}
	return s.ports.RuntimeConfigIssues(overrides)
}

// bindingConfigIssues reports the trigger bindings that point at an agent
// that cannot run them (missing, retired, paused or a managed sub-agent that
// may not be called directly).
func (s *Scanner) bindingConfigIssues(bySlug map[string]roster.Profile) map[string][]api.ConfigIssue {
	out := map[string][]api.ConfigIssue{}
	add := func(slug, key, issue string) {
		slug = strings.TrimSpace(slug)
		if slug == "" {
			return
		}
		out[slug] = append(out[slug], api.ConfigIssue{Key: key, Issue: issue})
	}
	if s.ports.Schedules != nil {
		for _, b := range s.ports.Schedules() {
			slug := strings.TrimSpace(b.Agent)
			if slug == "" {
				continue
			}
			p, ok := bySlug[slug]
			key := "schedule:" + b.ID
			if !ok {
				add(slug, key, "bound schedule targets missing agent")
				continue
			}
			if p.Retired {
				add(slug, key, "bound schedule targets retired agent")
				continue
			}
			if !p.Enabled {
				add(slug, key, "bound schedule targets paused agent")
				continue
			}
			if !p.AllowsDirectCall() {
				add(slug, key, "bound schedule cannot call managed sub-agent")
			}
		}
	}
	if s.ports.Standing != nil {
		for _, b := range s.ports.Standing() {
			slug := strings.TrimSpace(b.Agent)
			if slug == "" {
				continue
			}
			p, ok := bySlug[slug]
			key := "standing:" + b.ID
			if !ok {
				add(slug, key, "bound standing order targets missing agent")
				continue
			}
			if p.Retired {
				add(slug, key, "bound standing order targets retired agent")
				continue
			}
			if !p.Enabled {
				add(slug, key, "bound standing order targets paused agent")
				continue
			}
			if !p.AllowsDirectCall() {
				add(slug, key, "bound standing order cannot call managed sub-agent")
			}
		}
	}
	return out
}

// hierarchyConfigIssues reports an agent whose owner/parent references cannot
// hold: no owner at all for a managed sub-agent, a self-reference, or a
// reference to a missing, retired or paused agent.
func hierarchyConfigIssues(p roster.Profile, bySlug map[string]roster.Profile) []api.ConfigIssue {
	var issues []api.ConfigIssue
	if !p.AllowsDirectCall() && strings.TrimSpace(p.OwnerAgent) == "" && strings.TrimSpace(p.ParentAgent) == "" {
		issues = append(issues, api.ConfigIssue{Key: "hierarchy", Issue: "managed sub-agent has no owner_agent or parent_agent"})
	}
	for label, ref := range map[string]string{"owner_agent": p.OwnerAgent, "parent_agent": p.ParentAgent} {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if strings.EqualFold(ref, strings.TrimSpace(p.Slug)) {
			issues = append(issues, api.ConfigIssue{Key: label, Issue: "points to itself"})
			continue
		}
		manager, ok := bySlug[ref]
		if !ok {
			issues = append(issues, api.ConfigIssue{Key: label, Issue: ref + " is missing from the roster"})
			continue
		}
		if manager.Retired {
			issues = append(issues, api.ConfigIssue{Key: label, Issue: ref + " is retired"})
			continue
		}
		if !manager.Enabled {
			issues = append(issues, api.ConfigIssue{Key: label, Issue: ref + " is paused"})
		}
	}
	return issues
}
