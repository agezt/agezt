// SPDX-License-Identifier: MIT

package reaper

// Reaper: routing pressure + retry pressure (the "what's straining" cluster).

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

// routingPressureAgents finds the agents whose recent per-task model-chain
// fallbacks crossed the routing pressure threshold.
func (s *Scanner) routingPressureAgents(profiles []roster.Profile, cutoffMS int64) []api.RoutingPressureAgent {
	threshold := routingPressureThreshold()
	if threshold <= 0 {
		return nil
	}
	windowSec := int(routingPressureWindow() / time.Second)
	counts := map[string]int{}
	lastBySlug := map[string]api.RoutingPressureAgent{}
	bySlug := make(map[string]roster.Profile, len(profiles))
	for _, p := range profiles {
		bySlug[p.Slug] = p
	}
	if s.ports.Journal != nil {
		_ = s.ports.Journal(func(e *event.Event) error {
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
					cur = api.RoutingPressureAgent{
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
	}
	var out []api.RoutingPressureAgent
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

// routingPressureMatchesProfile reports whether a model-chain fallback concerns
// this agent: its task type matches, or one of the fallback models is in its
// chain.
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

// routingPressureModelChain is the agent's own model chain: its primary model
// followed by its fallbacks, de-duplicated.
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

// retryPressureAgents finds the agents whose recent runs hit transient
// provider failures often enough to cross the retry-pressure threshold.
func (s *Scanner) retryPressureAgents(profiles []roster.Profile, cutoffMS int64) []api.RetryPressureAgent {
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
	lastBySlug := map[string]api.RetryPressureAgent{}
	if s.ports.Journal != nil {
		_ = s.ports.Journal(func(e *event.Event) error {
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
				cur = api.RetryPressureAgent{
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
	}
	var out []api.RetryPressureAgent
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

// agentSlugFromRetrySubject reads the agent slug out of a retry event's
// subject ("agent.<slug>.retry") when the payload does not carry one.
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

// retryPressureThreshold is how many retries in the window count as pressure.
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

// retryPressureWindow is how far back the retry pressure is counted.
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

// routingDoctorAgent is the agent's doctor, if it has one.
func routingDoctorAgent(p roster.Profile) string {
	if p.HealthPolicy != nil {
		return p.HealthPolicy.DoctorAgent
	}
	return ""
}

// routingPressureThreshold is how many model-chain fallbacks count as
// pressure.
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

// routingPressureWindow is how far back the routing pressure is counted.
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

// routingForceProbationWindow is how long a forced chain stays on probation.
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
