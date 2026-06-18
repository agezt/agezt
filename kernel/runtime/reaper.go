// SPDX-License-Identifier: MIT

package runtime

// The reaper (#53) finds what's gone stale — agents that look abandoned and
// artifacts past their useful life — so they can be retired to the graveyard
// (roster.SetRetired, M846) or collected (artifact Collect, M845). This file is
// the DETECTION half: read-only scans. It mutates nothing — retire/collect stay
// operator-gated. The pulse ReaperObserver (kernel/pulse) runs ReaperScan on a
// cadence to surface candidates autonomously; the control plane exposes it
// on-demand for `agt reaper` and the UI.

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
)

// ReaperAgent is a dead-agent candidate: an enabled, non-retired roster agent,
// old enough to judge, with no task activity since the idle cutoff.
type ReaperAgent struct {
	Slug         string `json:"slug"`
	Name         string `json:"name,omitempty"`
	LastActiveMS int64  `json:"last_active_ms"` // 0 = never ran a task
}

// DegradedAgent is an enabled roster agent whose recent terminal runs crossed
// its profile health threshold. Detection only: repair/escalation remains a
// separate action by a guardian/doctor or operator.
type DegradedAgent struct {
	Slug              string `json:"slug"`
	Name              string `json:"name,omitempty"`
	Failures          int    `json:"failures"`
	Window            int    `json:"window"`
	Threshold         int    `json:"threshold"`
	DoctorAgent       string `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool   `json:"self_repair_enabled,omitempty"`
	EscalateTo        string `json:"escalate_to,omitempty"`
	LastFailureMS     int64  `json:"last_failure_ms,omitempty"`
	LastReason        string `json:"last_reason,omitempty"`
}

// MisconfiguredAgent is an enabled, non-retired agent whose runtime override
// config or hierarchy references are invalid, so one or more intended autonomy
// knobs will not actually apply until repaired.
type MisconfiguredAgent struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name,omitempty"`
	Issues            []string `json:"issues,omitempty"`
	DoctorAgent       string   `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool     `json:"self_repair_enabled,omitempty"`
	EscalateTo        string   `json:"escalate_to,omitempty"`
}

// RoutingPressureAgent is an enabled, non-retired agent whose recent per-task
// model-chain fallbacks crossed the routing pressure threshold. This catches
// agents that are limping along by repeatedly dropping to backup models even if
// they have not yet crossed the failed-run health threshold.
type RoutingPressureAgent struct {
	Slug              string `json:"slug"`
	Name              string `json:"name,omitempty"`
	Count             int    `json:"count"`
	Threshold         int    `json:"threshold"`
	WindowSec         int    `json:"window_sec"`
	DoctorAgent       string `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool   `json:"self_repair_enabled,omitempty"`
	EscalateTo        string `json:"escalate_to,omitempty"`
	LastFallbackMS    int64  `json:"last_fallback_ms,omitempty"`
	LastReason        string `json:"last_reason,omitempty"`
	LastFailedModel   string `json:"last_failed_model,omitempty"`
	LastNextModel     string `json:"last_next_model,omitempty"`
	TaskType          string `json:"task_type,omitempty"`
}

// RetryPressureAgent is an enabled, non-retired agent whose whole-run retry
// policy is being exercised repeatedly. This is separate from failed terminal
// runs: it catches agents that are still trying to recover before they fully
// fail the health window.
type RetryPressureAgent struct {
	Slug              string `json:"slug"`
	Name              string `json:"name,omitempty"`
	Count             int    `json:"count"`
	Threshold         int    `json:"threshold"`
	WindowSec         int    `json:"window_sec"`
	DoctorAgent       string `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool   `json:"self_repair_enabled,omitempty"`
	EscalateTo        string `json:"escalate_to,omitempty"`
	LastRetryMS       int64  `json:"last_retry_ms,omitempty"`
	LastReason        string `json:"last_reason,omitempty"`
	NextAttempt       int    `json:"next_attempt,omitempty"`
	MaxAttempts       int    `json:"max_attempts,omitempty"`
}

// RoutingForcedProbationAgent is an enabled, non-retired agent that is still
// under model-chain fallback pressure, but a manager/owner recently forced a
// specific routing chain and that same chain is still active. The doctor layer
// should observe the probation window before retuning it again.
type RoutingForcedProbationAgent struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name,omitempty"`
	Count             int      `json:"count"`
	Threshold         int      `json:"threshold"`
	WindowSec         int      `json:"window_sec"`
	DoctorAgent       string   `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool     `json:"self_repair_enabled,omitempty"`
	EscalateTo        string   `json:"escalate_to,omitempty"`
	LastFallbackMS    int64    `json:"last_fallback_ms,omitempty"`
	LastForcedMS      int64    `json:"last_forced_ms,omitempty"`
	LastReason        string   `json:"last_reason,omitempty"`
	TaskType          string   `json:"task_type,omitempty"`
	ForcedChain       []string `json:"forced_chain,omitempty"`
	ForceGeneration   int      `json:"routing_force_generation,omitempty"`
}

type RoutingForcedFailedAgent struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name,omitempty"`
	Count             int      `json:"count"`
	Threshold         int      `json:"threshold"`
	WindowSec         int      `json:"window_sec"`
	DoctorAgent       string   `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool     `json:"self_repair_enabled,omitempty"`
	EscalateTo        string   `json:"escalate_to,omitempty"`
	LastFallbackMS    int64    `json:"last_fallback_ms,omitempty"`
	LastForcedMS      int64    `json:"last_forced_ms,omitempty"`
	LastReason        string   `json:"last_reason,omitempty"`
	TaskType          string   `json:"task_type,omitempty"`
	ForcedChain       []string `json:"forced_chain,omitempty"`
	ForceGeneration   int      `json:"routing_force_generation,omitempty"`
}

type RoutingForcedExhaustedAgent struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name,omitempty"`
	Count             int      `json:"count"`
	Threshold         int      `json:"threshold"`
	WindowSec         int      `json:"window_sec"`
	DoctorAgent       string   `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool     `json:"self_repair_enabled,omitempty"`
	EscalateTo        string   `json:"escalate_to,omitempty"`
	LastFallbackMS    int64    `json:"last_fallback_ms,omitempty"`
	LastForcedMS      int64    `json:"last_forced_ms,omitempty"`
	LastReason        string   `json:"last_reason,omitempty"`
	TaskType          string   `json:"task_type,omitempty"`
	ForcedChain       []string `json:"forced_chain,omitempty"`
	ForceGeneration   int      `json:"routing_force_generation,omitempty"`
}

// RoutingUnstableAgent is an enabled, non-retired agent whose routing repair
// loop already rolled back recently and is still under model-chain fallback
// pressure for the same task type. This is a stronger signal than plain
// routing pressure: self-repair already tried to retune the chain and the
// chain still destabilized again.
type RoutingUnstableAgent struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name,omitempty"`
	Count             int      `json:"count"`
	Threshold         int      `json:"threshold"`
	WindowSec         int      `json:"window_sec"`
	DoctorAgent       string   `json:"doctor_agent,omitempty"`
	SelfRepairEnabled bool     `json:"self_repair_enabled,omitempty"`
	EscalateTo        string   `json:"escalate_to,omitempty"`
	LastRollbackMS    int64    `json:"last_rollback_ms,omitempty"`
	TaskType          string   `json:"task_type,omitempty"`
	CurrentChain      []string `json:"current_chain,omitempty"`
	PreviousChain     []string `json:"previous_chain,omitempty"`
	LastReason        string   `json:"last_reason,omitempty"`
}

// ReaperReport is the read-only result of a scan: dead-agent candidates plus
// stale-artifact totals. Detection only.
type ReaperReport struct {
	DeadAgents             []ReaperAgent                 `json:"dead_agents"`
	DegradedAgents         []DegradedAgent               `json:"degraded_agents,omitempty"`
	MisconfiguredAgents    []MisconfiguredAgent          `json:"misconfigured_agents,omitempty"`
	RetryPressure          []RetryPressureAgent          `json:"retry_pressure_agents,omitempty"`
	RoutingPressure        []RoutingPressureAgent        `json:"routing_pressure_agents,omitempty"`
	RoutingForced          []RoutingForcedProbationAgent `json:"routing_forced_probation_agents,omitempty"`
	RoutingForcedFailed    []RoutingForcedFailedAgent    `json:"routing_forced_failed_agents,omitempty"`
	RoutingForcedExhausted []RoutingForcedExhaustedAgent `json:"routing_forced_exhausted_agents,omitempty"`
	RoutingUnstable        []RoutingUnstableAgent        `json:"routing_unstable_agents,omitempty"`
	StaleArtifacts         int                           `json:"stale_artifacts"`
	StaleBytes             int64                         `json:"stale_bytes"`
}

// Empty reports whether the scan found nothing to reap.
func (r ReaperReport) Empty() bool {
	return len(r.DeadAgents) == 0 && len(r.DegradedAgents) == 0 && len(r.MisconfiguredAgents) == 0 && len(r.RetryPressure) == 0 && len(r.RoutingPressure) == 0 && len(r.RoutingForced) == 0 && len(r.RoutingForcedFailed) == 0 && len(r.RoutingForcedExhausted) == 0 && len(r.RoutingUnstable) == 0 && r.StaleArtifacts == 0
}

// ReaperScan finds dead-agent candidates (enabled, non-retired, created before
// agentIdleCutoffMs, and with no task activity at/after it) and counts stale
// artifacts (created before artifactStaleCutoffMs). Both cutoffs are absolute
// wall-clock ms — the caller passes `now - grace`. Read-only.
//
// "Created before the cutoff" is a grace window: a freshly-added agent that
// hasn't run yet is not reaped until it's been idle past the threshold, so the
// scan never flags an agent the operator just set up.
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

func (k *Kernel) routingPressureAgents(profiles []roster.Profile, cutoffMS int64) []RoutingPressureAgent {
	threshold := routingPressureThreshold()
	if threshold <= 0 {
		return nil
	}
	windowSec := int(routingPressureWindow() / time.Second)
	counts := map[string]int{}
	lastBySlug := map[string]RoutingPressureAgent{}
	bySlug := make(map[string]roster.Profile, len(profiles))
	for _, p := range profiles {
		bySlug[p.Slug] = p
	}
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindProviderFallback || e.TSUnixMS < cutoffMS {
			return nil
		}
		var pl struct {
			FailedModel string `json:"failed_model"`
			NextModel   string `json:"next_model"`
			Reason      string `json:"reason"`
			Scope       string `json:"scope"`
			TaskType    string `json:"task_type"`
		}
		if json.Unmarshal(e.Payload, &pl) != nil || strings.TrimSpace(pl.Scope) != "model-chain" {
			return nil
		}
		for slug, p := range bySlug {
			if !routingPressureMatchesProfile(p, pl.TaskType, pl.FailedModel, pl.NextModel) {
				continue
			}
			counts[slug]++
			cur := lastBySlug[slug]
			if e.TSUnixMS >= cur.LastFallbackMS {
				cur = RoutingPressureAgent{
					Slug:            p.Slug,
					Name:            p.Name,
					Threshold:       threshold,
					WindowSec:       windowSec,
					DoctorAgent:     routingDoctorAgent(p),
					LastFallbackMS:  e.TSUnixMS,
					LastReason:      strings.TrimSpace(pl.Reason),
					LastFailedModel: strings.TrimSpace(pl.FailedModel),
					LastNextModel:   strings.TrimSpace(pl.NextModel),
					TaskType:        strings.TrimSpace(pl.TaskType),
				}
				if p.SelfRepairPolicy != nil {
					cur.SelfRepairEnabled = p.SelfRepairPolicy.Enabled
					cur.EscalateTo = p.SelfRepairPolicy.EscalateTo
				}
				lastBySlug[slug] = cur
			}
		}
		return nil
	})
	var out []RoutingPressureAgent
	for slug, count := range counts {
		if count < threshold {
			continue
		}
		row := lastBySlug[slug]
		row.Count = count
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

func routingPressureMatchesProfile(p roster.Profile, taskType, failedModel, nextModel string) bool {
	if !p.Enabled || p.Retired {
		return false
	}
	taskType = strings.TrimSpace(taskType)
	failedModel = strings.TrimSpace(failedModel)
	nextModel = strings.TrimSpace(nextModel)
	if pt := strings.TrimSpace(p.TaskType); pt != "" && strings.EqualFold(pt, taskType) {
		return true
	}
	for _, model := range routingPressureModelChain(p) {
		if strings.EqualFold(model, failedModel) || strings.EqualFold(model, nextModel) {
			return true
		}
	}
	return false
}

func routingPressureModelChain(p roster.Profile) []string {
	var out []string
	seen := map[string]bool{}
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" || seen[strings.ToLower(model)] {
			return
		}
		seen[strings.ToLower(model)] = true
		out = append(out, model)
	}
	add(p.Model)
	for _, model := range p.Fallbacks {
		add(model)
	}
	return out
}

func (k *Kernel) retryPressureAgents(profiles []roster.Profile, cutoffMS int64) []RetryPressureAgent {
	threshold := retryPressureThreshold()
	if threshold <= 0 {
		return nil
	}
	windowSec := int(retryPressureWindow() / time.Second)
	bySlug := make(map[string]roster.Profile, len(profiles))
	for _, p := range profiles {
		bySlug[p.Slug] = p
	}
	counts := map[string]int{}
	lastBySlug := map[string]RetryPressureAgent{}
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindAgentRetry || e.TSUnixMS < cutoffMS {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		slug := strings.TrimSpace(plStringAny(pl["agent"]))
		if slug == "" {
			slug = agentSlugFromRetrySubject(e.Subject)
		}
		p, ok := bySlug[slug]
		if !ok || !p.Enabled || p.Retired {
			return nil
		}
		counts[slug]++
		cur := lastBySlug[slug]
		if e.TSUnixMS >= cur.LastRetryMS {
			cur = RetryPressureAgent{
				Slug:        p.Slug,
				Name:        p.Name,
				Threshold:   threshold,
				WindowSec:   windowSec,
				DoctorAgent: routingDoctorAgent(p),
				LastRetryMS: e.TSUnixMS,
				LastReason:  strings.TrimSpace(plStringAny(pl["reason"])),
				NextAttempt: plIntAny(pl["next_attempt"]),
				MaxAttempts: plIntAny(pl["max_attempts"]),
			}
			if p.SelfRepairPolicy != nil {
				cur.SelfRepairEnabled = p.SelfRepairPolicy.Enabled
				cur.EscalateTo = p.SelfRepairPolicy.EscalateTo
			}
			lastBySlug[slug] = cur
		}
		return nil
	})
	var out []RetryPressureAgent
	for slug, count := range counts {
		if count < threshold {
			continue
		}
		row := lastBySlug[slug]
		row.Count = count
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

func agentSlugFromRetrySubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if !strings.HasPrefix(subject, "agent.") || !strings.HasSuffix(subject, ".retry") {
		return ""
	}
	slug := strings.TrimSuffix(strings.TrimPrefix(subject, "agent."), ".retry")
	if slug == "" || slug == "retry" {
		return ""
	}
	return slug
}

func retryPressureThreshold() int {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "RETRY_PRESSURE_THRESHOLD"))
	if raw == "" {
		return 3
	}
	if n := parsePositiveInt(raw); n > 0 {
		return n
	}
	return 3
}

func retryPressureWindow() time.Duration {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "RETRY_PRESSURE_WINDOW"))
	if raw == "" {
		return 24 * time.Hour
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 24 * time.Hour
	}
	return d
}

func routingDoctorAgent(p roster.Profile) string {
	if p.HealthPolicy != nil {
		return p.HealthPolicy.DoctorAgent
	}
	return ""
}

func routingPressureThreshold() int {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ROUTING_PRESSURE_THRESHOLD"))
	if raw == "" {
		return 3
	}
	if n := parsePositiveInt(raw); n > 0 {
		return n
	}
	return 3
}

func routingPressureWindow() time.Duration {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ROUTING_PRESSURE_WINDOW"))
	if raw == "" {
		return 24 * time.Hour
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 24 * time.Hour
	}
	return d
}

func routingForceProbationWindow() time.Duration {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ROUTING_FORCE_PROBATION"))
	if raw == "" {
		return 4 * time.Hour
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 4 * time.Hour
	}
	return d
}

func (k *Kernel) routingForcedProbationAgents(profiles []roster.Profile, routing []RoutingPressureAgent, cutoffMS int64) ([]RoutingForcedProbationAgent, []RoutingForcedFailedAgent, []RoutingForcedExhaustedAgent, []RoutingPressureAgent) {
	if len(routing) == 0 {
		return nil, nil, nil, nil
	}
	type forcedRow struct {
		TSMS       int64
		TaskType   string
		Chain      []string
		AgentSlug  string
		Generation int
	}
	latestForced := map[string]forcedRow{}
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindInfo || (e.Subject != "doctor.auto_repair" && e.Subject != "agent.resolve") {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		phase := strings.TrimSpace(plStringAny(pl["phase"]))
		if phase != "resolution_applied" && phase != "completed" || strings.TrimSpace(plStringAny(pl["resolution"])) != "force_chain" {
			return nil
		}
		slug := strings.TrimSpace(plStringAny(pl["agent"]))
		taskType := strings.TrimSpace(plStringAny(pl["routing_task_type"]))
		chain := plStringsAny(pl["routing_task_model_chain"])
		if slug == "" || taskType == "" || len(chain) == 0 {
			return nil
		}
		gen := plIntAny(pl["routing_force_generation"])
		if gen <= 0 {
			gen = 1
		}
		key := slug + "::" + taskType
		cur, ok := latestForced[key]
		if !ok || e.TSUnixMS >= cur.TSMS {
			latestForced[key] = forcedRow{TSMS: e.TSUnixMS, TaskType: taskType, Chain: chain, AgentSlug: slug, Generation: gen}
		}
		return nil
	})
	bySlug := make(map[string]roster.Profile, len(profiles))
	for _, p := range profiles {
		bySlug[p.Slug] = p
	}
	var probation []RoutingForcedProbationAgent
	var failed []RoutingForcedFailedAgent
	var exhausted []RoutingForcedExhaustedAgent
	var active []RoutingPressureAgent
	for _, row := range routing {
		p := bySlug[row.Slug]
		taskType := strings.TrimSpace(row.TaskType)
		if taskType == "" {
			taskType = strings.TrimSpace(p.TaskType)
		}
		if taskType == "" {
			active = append(active, row)
			continue
		}
		forced, ok := latestForced[row.Slug+"::"+taskType]
		currentChain := k.currentTaskModelChain(taskType)
		if len(currentChain) == 0 {
			currentChain = routingPressureModelChain(p)
		}
		if !ok || !sameTaskModelChain(currentChain, forced.Chain) {
			active = append(active, row)
			continue
		}
		if forced.TSMS >= cutoffMS {
			probation = append(probation, RoutingForcedProbationAgent{
				Slug:              row.Slug,
				Name:              p.Name,
				Count:             row.Count,
				Threshold:         row.Threshold,
				WindowSec:         row.WindowSec,
				DoctorAgent:       row.DoctorAgent,
				SelfRepairEnabled: row.SelfRepairEnabled,
				EscalateTo:        row.EscalateTo,
				LastFallbackMS:    row.LastFallbackMS,
				LastForcedMS:      forced.TSMS,
				LastReason:        row.LastReason,
				TaskType:          taskType,
				ForcedChain:       append([]string(nil), forced.Chain...),
				ForceGeneration:   forced.Generation,
			})
			continue
		}
		if forced.Generation >= 2 {
			exhausted = append(exhausted, RoutingForcedExhaustedAgent{
				Slug:              row.Slug,
				Name:              p.Name,
				Count:             row.Count,
				Threshold:         row.Threshold,
				WindowSec:         row.WindowSec,
				DoctorAgent:       row.DoctorAgent,
				SelfRepairEnabled: row.SelfRepairEnabled,
				EscalateTo:        row.EscalateTo,
				LastFallbackMS:    row.LastFallbackMS,
				LastForcedMS:      forced.TSMS,
				LastReason:        row.LastReason,
				TaskType:          taskType,
				ForcedChain:       append([]string(nil), forced.Chain...),
				ForceGeneration:   forced.Generation,
			})
			continue
		}
		failed = append(failed, RoutingForcedFailedAgent{
			Slug:              row.Slug,
			Name:              p.Name,
			Count:             row.Count,
			Threshold:         row.Threshold,
			WindowSec:         row.WindowSec,
			DoctorAgent:       row.DoctorAgent,
			SelfRepairEnabled: row.SelfRepairEnabled,
			EscalateTo:        row.EscalateTo,
			LastFallbackMS:    row.LastFallbackMS,
			LastForcedMS:      forced.TSMS,
			LastReason:        row.LastReason,
			TaskType:          taskType,
			ForcedChain:       append([]string(nil), forced.Chain...),
			ForceGeneration:   forced.Generation,
		})
	}
	sort.Slice(probation, func(i, j int) bool { return probation[i].Slug < probation[j].Slug })
	sort.Slice(failed, func(i, j int) bool { return failed[i].Slug < failed[j].Slug })
	sort.Slice(exhausted, func(i, j int) bool { return exhausted[i].Slug < exhausted[j].Slug })
	sort.Slice(active, func(i, j int) bool { return active[i].Slug < active[j].Slug })
	return probation, failed, exhausted, active
}

func (k *Kernel) routingUnstableAgents(profiles []roster.Profile, routing []RoutingPressureAgent, cutoffMS int64) []RoutingUnstableAgent {
	threshold := routingUnstableThreshold()
	if threshold <= 0 || len(routing) == 0 {
		return nil
	}
	windowSec := int(routingUnstableWindow() / time.Second)
	type row struct {
		Count          int
		LastRollbackMS int64
		TaskType       string
		CurrentChain   []string
		PreviousChain  []string
		LastReason     string
	}
	pressureByKey := map[string]RoutingPressureAgent{}
	for _, p := range routing {
		taskType := strings.TrimSpace(p.TaskType)
		if taskType == "" {
			taskType = "*"
		}
		pressureByKey[p.Slug+"::"+taskType] = p
		if taskType != "*" {
			if _, ok := pressureByKey[p.Slug+"::*"]; !ok {
				pressureByKey[p.Slug+"::*"] = p
			}
		}
	}
	counts := map[string]row{}
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.TSUnixMS < cutoffMS || e.Kind != event.KindInfo {
			return nil
		}
		if e.Subject != "doctor.auto_repair" && e.Subject != "agent.repair" {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil || strings.TrimSpace(plStringAny(pl["phase"])) != "routing_rollback_completed" {
			return nil
		}
		slug := strings.TrimSpace(plStringAny(pl["agent"]))
		if slug == "" {
			return nil
		}
		taskType := strings.TrimSpace(plStringAny(pl["routing_task_type"]))
		pressure, ok := pressureByKey[slug+"::"+taskType]
		if !ok {
			pressure, ok = pressureByKey[slug+"::*"]
			if !ok {
				return nil
			}
			if taskType == "" {
				taskType = strings.TrimSpace(pressure.TaskType)
			}
		}
		key := slug + "::" + taskType
		cur := counts[key]
		cur.Count++
		if e.TSUnixMS >= cur.LastRollbackMS {
			cur.LastRollbackMS = e.TSUnixMS
			cur.TaskType = taskType
			cur.CurrentChain = plStringsAny(pl["routing_task_model_chain"])
			cur.PreviousChain = plStringsAny(pl["previous_routing_task_model_chain"])
			cur.LastReason = strings.TrimSpace(pressure.LastReason)
		}
		counts[key] = cur
		return nil
	})
	bySlug := make(map[string]roster.Profile, len(profiles))
	for _, p := range profiles {
		bySlug[p.Slug] = p
	}
	var out []RoutingUnstableAgent
	for key, meta := range counts {
		if meta.Count < threshold {
			continue
		}
		parts := strings.SplitN(key, "::", 2)
		slug := parts[0]
		p, ok := bySlug[slug]
		if !ok {
			continue
		}
		row := RoutingUnstableAgent{
			Slug:           slug,
			Name:           p.Name,
			Count:          meta.Count,
			Threshold:      threshold,
			WindowSec:      windowSec,
			LastRollbackMS: meta.LastRollbackMS,
			TaskType:       meta.TaskType,
			CurrentChain:   meta.CurrentChain,
			PreviousChain:  meta.PreviousChain,
			LastReason:     meta.LastReason,
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
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

func routingUnstableThreshold() int {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ROUTING_UNSTABLE_THRESHOLD"))
	if raw == "" {
		return 1
	}
	if n := parsePositiveInt(raw); n > 0 {
		return n
	}
	return 1
}

func routingUnstableWindow() time.Duration {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ROUTING_UNSTABLE_WINDOW"))
	if raw == "" {
		return 6 * time.Hour
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 6 * time.Hour
	}
	return d
}

func (k *Kernel) currentTaskModelChain(taskType string) []string {
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		return nil
	}
	type taskModelChainsSource interface {
		TaskModelChainsView() map[string][]string
	}
	gov, ok := k.Provider().(taskModelChainsSource)
	if !ok {
		return nil
	}
	src := gov.TaskModelChainsView()[taskType]
	if len(src) == 0 {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

func sameTaskModelChain(a, b []string) bool {
	if len(a) != len(b) || len(a) == 0 {
		return false
	}
	for i := range a {
		if !strings.EqualFold(strings.TrimSpace(a[i]), strings.TrimSpace(b[i])) {
			return false
		}
	}
	return true
}

func plStringAny(v any) string {
	s, _ := v.(string)
	return s
}

func plStringsAny(v any) []string {
	raw, ok := v.([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func plIntAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func parsePositiveInt(raw string) int {
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
