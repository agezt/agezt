// SPDX-License-Identifier: MIT

// Reaper scan entrypoint (ReaperScan + agentRunHealth).
// Code extracted from reaper_scan.go during the Day-91 god-file split.
// Public API unchanged.
package runtime


import (
	"sort"
	"time"

	"encoding/json"
	"github.com/agezt/agezt/kernel/event"
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

