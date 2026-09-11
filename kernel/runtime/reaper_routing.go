// SPDX-License-Identifier: MIT

package runtime

// Routing/retry-pressure agents scans + helpers: routingPressureAgents +
// routingPressureMatchesProfile + routingPressureModelChain +
// retryPressureAgents + agentSlugFromRetrySubject +
// retryPressureThreshold/Window + routingDoctorAgent +
// routingPressureThreshold/Window + routingForceProbationWindow +
// routingForcedProbationAgents + routingUnstableAgents +
// routingUnstableThreshold/Window. Carved out of reaper.go during
// the Day 34 god file split #1.

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

