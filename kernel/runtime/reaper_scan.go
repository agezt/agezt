// SPDX-License-Identifier: MIT

package runtime

// ReaperScan: the main scan that walks the journal + profiles and emits a
// ReaperReport. Includes degradedAgents + misconfiguredAgents +
// agentBindingConfigIssues + agentHierarchyConfigIssues. Carved out of
// reaper.go during the Day 34 god file split #1.

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
)

func (k *Kernel) ReaperScan(agentIdleCutoffMs, artifactStaleCutoffMs int64) ReaperReport {
	// Last task.received timestamp per agent slug — task.received carries the
	// acting agent's slug since M854 (same source the activity log uses).
	lastActive := map[string]int64{}
	runAgent := map[string]string{}
	runs := map[string]agentRunHealth{}
	_ = k.Journal().Range(func(e *event.Event) error {
		var pl map[string]any
		_ = json.Unmarshal(e.Payload, &pl)
		switch e.Kind {
		case event.KindTaskReceived:
			if slug, _ := pl["agent"].(string); slug != "" {
				if e.TSUnixMS > lastActive[slug] {
					lastActive[slug] = e.TSUnixMS
				}
				runAgent[e.CorrelationID] = slug
				r := runs[e.CorrelationID]
				r.agent = slug
				r.startedMS = e.TSUnixMS
				runs[e.CorrelationID] = r
			}
		case event.KindTaskCompleted:
			if slug := runAgent[e.CorrelationID]; slug != "" {
				r := runs[e.CorrelationID]
				r.agent = slug
				r.status = "completed"
				r.endedMS = e.TSUnixMS
				runs[e.CorrelationID] = r
			}
		case event.KindTaskFailed:
			if slug := runAgent[e.CorrelationID]; slug != "" {
				r := runs[e.CorrelationID]
				r.agent = slug
				r.status = "failed"
				r.endedMS = e.TSUnixMS
				r.reason, _ = pl["reason"].(string)
				runs[e.CorrelationID] = r
			}
		}
		return nil
	})

	var dead []ReaperAgent
	profiles := k.Roster().List()
	for _, p := range profiles {
		if !p.Enabled || p.Retired {
			continue // paused/retired agents aren't "dead", just inactive on purpose
		}
		if p.System {
			continue // shipped guardians are long-lived by design — never reap them (M961)
		}
		if p.CreatedMS == 0 || p.CreatedMS >= agentIdleCutoffMs {
			continue // too new to judge (within the grace window)
		}
		if last := lastActive[p.Slug]; last >= agentIdleCutoffMs {
			continue // ran a task recently enough
		}
		dead = append(dead, ReaperAgent{Slug: p.Slug, Name: p.Name, LastActiveMS: lastActive[p.Slug]})
	}
	sort.Slice(dead, func(i, j int) bool { return dead[i].Slug < dead[j].Slug })

	degraded := degradedAgents(profiles, runs)
	misconfigured := k.misconfiguredAgents(profiles)
	retryPressure := k.retryPressureAgents(profiles, time.Now().Add(-retryPressureWindow()).UnixMilli())
	routingPressureAll := k.routingPressureAgents(profiles, time.Now().Add(-routingPressureWindow()).UnixMilli())
	routingForced, routingForcedFailed, routingForcedExhausted, routingPressure := k.routingForcedProbationAgents(profiles, routingPressureAll, time.Now().Add(-routingForceProbationWindow()).UnixMilli())
	routingUnstable := k.routingUnstableAgents(profiles, routingPressure, time.Now().Add(-routingUnstableWindow()).UnixMilli())

	var staleN int
	var staleBytes int64
	if idx := k.ArtifactIndex(); idx != nil {
		for _, e := range idx.StaleEntries(artifactStaleCutoffMs) {
			staleN++
			staleBytes += e.Size
		}
	}
	return ReaperReport{
		DeadAgents:             dead,
		DegradedAgents:         degraded,
		MisconfiguredAgents:    misconfigured,
		RetryPressure:          retryPressure,
		RoutingPressure:        routingPressure,
		RoutingForced:          routingForced,
		RoutingForcedFailed:    routingForcedFailed,
		RoutingForcedExhausted: routingForcedExhausted,
		RoutingUnstable:        routingUnstable,
		StaleArtifacts:         staleN,
		StaleBytes:             staleBytes,
	}
}

type agentRunHealth struct {
	agent     string
	status    string
	reason    string
	startedMS int64
	endedMS   int64
}

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

