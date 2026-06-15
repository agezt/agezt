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
	"sort"

	"github.com/agezt/agezt/kernel/event"
)

// ReaperAgent is a dead-agent candidate: an enabled, non-retired roster agent,
// old enough to judge, with no task activity since the idle cutoff.
type ReaperAgent struct {
	Slug         string `json:"slug"`
	Name         string `json:"name,omitempty"`
	LastActiveMS int64  `json:"last_active_ms"` // 0 = never ran a task
}

// ReaperReport is the read-only result of a scan: dead-agent candidates plus
// stale-artifact totals. Detection only.
type ReaperReport struct {
	DeadAgents     []ReaperAgent `json:"dead_agents"`
	StaleArtifacts int           `json:"stale_artifacts"`
	StaleBytes     int64         `json:"stale_bytes"`
}

// Empty reports whether the scan found nothing to reap.
func (r ReaperReport) Empty() bool {
	return len(r.DeadAgents) == 0 && r.StaleArtifacts == 0
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
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindTaskReceived {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) == nil {
			if slug, _ := pl["agent"].(string); slug != "" && e.TSUnixMS > lastActive[slug] {
				lastActive[slug] = e.TSUnixMS
			}
		}
		return nil
	})

	var dead []ReaperAgent
	for _, p := range k.Roster().List() {
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

	var staleN int
	var staleBytes int64
	if idx := k.ArtifactIndex(); idx != nil {
		for _, e := range idx.StaleEntries(artifactStaleCutoffMs) {
			staleN++
			staleBytes += e.Size
		}
	}
	return ReaperReport{DeadAgents: dead, StaleArtifacts: staleN, StaleBytes: staleBytes}
}
