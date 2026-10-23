// SPDX-License-Identifier: MIT

package reaper

// Reaper: forced probation + unstable agents (the "what's on probation"
// cluster).

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/agents/api"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
)

// routingForcedProbationAgents splits the routing pressure rows into the
// agents whose chain was forced by a manager or owner: still on probation,
// failed after it, or exhausted by repeated forced generations. The rows whose
// chain no longer matches the forced one stay active pressure.
func (s *Scanner) routingForcedProbationAgents(profiles []roster.Profile, routing []api.RoutingPressureAgent, cutoffMS int64) ([]api.RoutingForcedProbationAgent, []api.RoutingForcedFailedAgent, []api.RoutingForcedExhaustedAgent, []api.RoutingPressureAgent) {
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
	if s.ports.Journal != nil {
		_ = s.ports.Journal(func(e *event.Event) error {
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
	}
	bySlug := make(map[string]roster.Profile, len(profiles))
	for _, p := range profiles {
		bySlug[p.Slug] = p
	}
	var probation []api.RoutingForcedProbationAgent
	var failed []api.RoutingForcedFailedAgent
	var exhausted []api.RoutingForcedExhaustedAgent
	var active []api.RoutingPressureAgent
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
		currentChain := s.currentTaskModelChain(taskType)
		if len(currentChain) == 0 {
			currentChain = routingPressureModelChain(p)
		}
		if !ok || !sameTaskModelChain(currentChain, forced.Chain) {
			active = append(active, row)
			continue
		}
		if forced.TSMS >= cutoffMS {
			probation = append(probation, api.RoutingForcedProbationAgent{
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
			exhausted = append(exhausted, api.RoutingForcedExhaustedAgent{
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
		failed = append(failed, api.RoutingForcedFailedAgent{
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

// routingUnstableAgents finds the agents whose routing repair loop already
// rolled back recently and are still under model-chain fallback pressure for
// the same task type.
func (s *Scanner) routingUnstableAgents(profiles []roster.Profile, routing []api.RoutingPressureAgent, cutoffMS int64) []api.RoutingUnstableAgent {
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
	pressureByKey := map[string]api.RoutingPressureAgent{}
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
	if s.ports.Journal != nil {
		_ = s.ports.Journal(func(e *event.Event) error {
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
	}
	bySlug := make(map[string]roster.Profile, len(profiles))
	for _, p := range profiles {
		bySlug[p.Slug] = p
	}
	var out []api.RoutingUnstableAgent
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
		row := api.RoutingUnstableAgent{
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

// currentTaskModelChain is the host's live model chain for a task type.
func (s *Scanner) currentTaskModelChain(taskType string) []string {
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		return nil
	}
	if s.ports.TaskModelChain == nil {
		return nil
	}
	src := s.ports.TaskModelChain(taskType)
	if len(src) == 0 {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// routingUnstableThreshold is how many rollbacks in the window count as an
// unstable chain.
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

// routingUnstableWindow is how far back the rollbacks are counted.
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
