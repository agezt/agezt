// SPDX-License-Identifier: MIT

// Package reaper is the agents module's reaper: the read-only DETECTION half
// of the fleet's health scan. It finds what's gone stale — agents that look
// abandoned and artifacts past their useful life — so they can be retired to
// the graveyard or collected. It mutates nothing: retire/collect stay
// operator-gated.
//
// The module holds no kernel of its own. Everything it reads arrives through
// the host's ports (api.ReaperPorts): the journal, the fleet, the host's
// runtime-config validation, the artifact index, the governor's live model
// chains and the trigger bindings. The pulse ReaperObserver runs a scan on a
// cadence to surface candidates autonomously; the control plane exposes it
// on-demand for `agt reaper` and the UI.
package reaper

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/agents/api"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
)

// Scanner is the reaper over one set of host ports.
type Scanner struct {
	ports api.ReaperPorts
}

// New builds the reaper over the host's ports.
func New(ports api.ReaperPorts) *Scanner { return &Scanner{ports: ports} }

// Scan implements api.ReaperScanner: it finds dead-agent candidates (enabled,
// non-retired, created before the agent idle cutoff, and with no task activity
// at/after it) and counts stale artifacts (created before the artifact stale
// cutoff). Both cutoffs are absolute wall-clock ms — the caller passes
// `now - grace`. Read-only.
//
// "Created before the cutoff" is a grace window: a freshly-added agent that
// hasn't run yet is not reaped until it's been idle past the threshold, so the
// scan never flags an agent the operator just set up.
func (s *Scanner) Scan(agentIdleCutoffMs, artifactStaleCutoffMs int64) api.ReaperReport {
	// Last task.received timestamp per agent slug — task.received carries the
	// acting agent's slug since M854 (same source the activity log uses).
	lastActive := map[string]int64{}
	runAgent := map[string]string{}
	runs := map[string]agentRunHealth{}
	if s.ports.Journal != nil {
		_ = s.ports.Journal(func(e *event.Event) error {
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
	}

	var dead []api.ReaperAgent
	profiles := s.profiles()
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
		dead = append(dead, api.ReaperAgent{Slug: p.Slug, Name: p.Name, LastActiveMS: lastActive[p.Slug]})
	}
	sort.Slice(dead, func(i, j int) bool { return dead[i].Slug < dead[j].Slug })

	degraded := degradedAgents(profiles, runs)
	misconfigured := s.misconfiguredAgents(profiles)
	retryPressure := s.retryPressureAgents(profiles, time.Now().Add(-retryPressureWindow()).UnixMilli())
	routingPressureAll := s.routingPressureAgents(profiles, time.Now().Add(-routingPressureWindow()).UnixMilli())
	routingForced, routingForcedFailed, routingForcedExhausted, routingPressure := s.routingForcedProbationAgents(profiles, routingPressureAll, time.Now().Add(-routingForceProbationWindow()).UnixMilli())
	routingUnstable := s.routingUnstableAgents(profiles, routingPressure, time.Now().Add(-routingUnstableWindow()).UnixMilli())

	var staleN int
	var staleBytes int64
	if s.ports.StaleArtifacts != nil {
		staleN, staleBytes = s.ports.StaleArtifacts(artifactStaleCutoffMs)
	}
	return api.ReaperReport{
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

// profiles lists the fleet the scan judges.
func (s *Scanner) profiles() []roster.Profile {
	if s.ports.Profiles == nil {
		return nil
	}
	return s.ports.Profiles()
}

// agentRunHealth is one agent's terminal run, folded from the journal.
type agentRunHealth struct {
	agent     string
	status    string
	reason    string
	startedMS int64
	endedMS   int64
}
