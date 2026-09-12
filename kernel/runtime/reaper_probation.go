// SPDX-License-Identifier: MIT

// Reaper: forced probation + unstable agents (the "what's on probation" cluster).
// Code extracted from reaper_routing.go during the Day-90 god-file split.
// Public API unchanged.
package runtime


import (
	"os"
	"sort"
	"strings"
	"time"

	"encoding/json"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
)

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

