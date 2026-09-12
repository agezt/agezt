// SPDX-License-Identifier: MIT

// Reaper: routing pressure + retry pressure (the "what's straining" cluster).
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

