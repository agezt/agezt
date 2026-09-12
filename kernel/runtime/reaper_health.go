// SPDX-License-Identifier: MIT

// Reaper health-classification helpers (degraded + misconfigured + binding/hierarchy config issues).
// Code extracted from reaper_scan.go during the Day-91 god-file split.
// Public API unchanged.
package runtime


import (
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/roster"
)

func degradedAgents(profiles []roster.Profile, runs map[string]agentRunHealth) []DegradedAgent {
	byAgent := map[string][]agentRunHealth{}
	for _, r := range runs {
		if r.agent == "" || r.status == "" {
			continue
		}
		byAgent[r.agent] = append(byAgent[r.agent], r)
	}
	var out []DegradedAgent
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
		row := DegradedAgent{
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

func (k *Kernel) misconfiguredAgents(profiles []roster.Profile) []MisconfiguredAgent {
	var out []MisconfiguredAgent
	bySlug := make(map[string]roster.Profile, len(profiles))
	for _, p := range profiles {
		bySlug[p.Slug] = p
	}
	bindingIssues := k.agentBindingConfigIssues(bySlug)
	seen := map[string]bool{}
	for _, p := range profiles {
		bindings := bindingIssues[p.Slug]
		if len(bindings) == 0 && (!p.Enabled || p.Retired) {
			continue
		}
		issues := agentRuntimeConfigIssues(p.ConfigOverrides)
		issues = append(issues, agentHierarchyConfigIssues(p, bySlug)...)
		issues = append(issues, bindings...)
		if len(issues) == 0 {
			continue
		}
		seen[p.Slug] = true
		row := MisconfiguredAgent{
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
		row := MisconfiguredAgent{Slug: slug}
		for _, issue := range issues {
			row.Issues = append(row.Issues, issue.Key+": "+issue.Issue)
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

func (k *Kernel) agentBindingConfigIssues(bySlug map[string]roster.Profile) map[string][]agentConfigIssue {
	out := map[string][]agentConfigIssue{}
	add := func(slug, key, issue string) {
		slug = strings.TrimSpace(slug)
		if slug == "" {
			return
		}
		out[slug] = append(out[slug], agentConfigIssue{Key: key, Issue: issue})
	}
	if sched := k.Schedules(); sched != nil {
		for _, e := range sched.List() {
			slug := strings.TrimSpace(e.Agent)
			if slug == "" {
				continue
			}
			p, ok := bySlug[slug]
			key := "schedule:" + e.ID
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
	if standing := k.Standing(); standing != nil {
		for _, o := range standing.List() {
			slug := strings.TrimSpace(o.Agent)
			if slug == "" {
				continue
			}
			p, ok := bySlug[slug]
			key := "standing:" + o.ID
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

func agentHierarchyConfigIssues(p roster.Profile, bySlug map[string]roster.Profile) []agentConfigIssue {
	var issues []agentConfigIssue
	if !p.AllowsDirectCall() && strings.TrimSpace(p.OwnerAgent) == "" && strings.TrimSpace(p.ParentAgent) == "" {
		issues = append(issues, agentConfigIssue{Key: "hierarchy", Issue: "managed sub-agent has no owner_agent or parent_agent"})
	}
	for label, ref := range map[string]string{"owner_agent": p.OwnerAgent, "parent_agent": p.ParentAgent} {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if strings.EqualFold(ref, strings.TrimSpace(p.Slug)) {
			issues = append(issues, agentConfigIssue{Key: label, Issue: "points to itself"})
			continue
		}
		manager, ok := bySlug[ref]
		if !ok {
			issues = append(issues, agentConfigIssue{Key: label, Issue: ref + " is missing from the roster"})
			continue
		}
		if manager.Retired {
			issues = append(issues, agentConfigIssue{Key: label, Issue: ref + " is retired"})
			continue
		}
		if !manager.Enabled {
			issues = append(issues, agentConfigIssue{Key: label, Issue: ref + " is paused"})
		}
	}
	return issues
}

