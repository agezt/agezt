// SPDX-License-Identifier: MIT

package pulse

import (
	"context"
	"fmt"
)

// ReaperScanFunc returns the current reaper/doctor candidate counts — dead
// agents, degraded live agents, misconfigured live agents, retry-pressure live
// agents, routing-pressure live agents, and stale artifacts. Injected so the observer is
// testable and pulse doesn't import the runtime; the daemon wires k.ReaperScan
// with its configured cutoffs (M903).
type ReaperScanFunc func() (deadAgents, degradedAgents, misconfiguredAgents, retryPressureAgents, routingPressureAgents, routingForcedProbationAgents, routingForcedFailedAgents, routingForcedExhaustedAgents, routingUnstableAgents, staleArtifacts int)

// ReaperObserver surfaces stale agents/artifacts on the pulse cadence (#53) —
// detection only; the operator still chooses to retire (graveyard) or collect.
// Transition-based like the disk observer: it fires when the pile of candidates
// first appears or GROWS, not every beat, so a standing backlog doesn't spam the
// briefing. Low severity — this is housekeeping, not an incident.
type ReaperObserver struct {
	scan                ReaperScanFunc
	lastDead            int
	lastDegraded        int
	lastMisconf         int
	lastRetry           int
	lastRouting         int
	lastForced          int
	lastForcedFailed    int
	lastForcedExhausted int
	lastUnstable        int
	lastArt             int
	hasState            bool
}

// NewReaperObserver constructs a reaper observer over the given scan.
func NewReaperObserver(scan ReaperScanFunc) *ReaperObserver {
	return &ReaperObserver{scan: scan}
}

// Name implements Observer.
func (o *ReaperObserver) Name() string { return "system:reaper" }

// Poll implements Observer.
func (o *ReaperObserver) Poll(_ context.Context) ([]Delta, error) {
	if o.scan == nil {
		return nil, nil
	}
	dead, degraded, misconf, retryPressure, routing, forced, forcedFailed, forcedExhausted, unstable, art := o.scan()

	prevDead, prevDegraded, prevMisconf, prevRetry, prevRouting, prevForced, prevForcedFailed, prevForcedExhausted, prevUnstable, prevArt, known := o.lastDead, o.lastDegraded, o.lastMisconf, o.lastRetry, o.lastRouting, o.lastForced, o.lastForcedFailed, o.lastForcedExhausted, o.lastUnstable, o.lastArt, o.hasState
	o.lastDead, o.lastDegraded, o.lastMisconf, o.lastRetry, o.lastRouting, o.lastForced, o.lastForcedFailed, o.lastForcedExhausted, o.lastUnstable, o.lastArt, o.hasState = dead, degraded, misconf, retryPressure, routing, forced, forcedFailed, forcedExhausted, unstable, art, true

	if !known {
		return nil, nil // baseline beat: establish state silently
	}
	// Fire only when something NEW went stale (the pile grew), and only when
	// there's actually something to reap now.
	if (dead <= prevDead && degraded <= prevDegraded && misconf <= prevMisconf && retryPressure <= prevRetry && routing <= prevRouting && forced <= prevForced && forcedFailed <= prevForcedFailed && forcedExhausted <= prevForcedExhausted && unstable <= prevUnstable && art <= prevArt) || (dead == 0 && degraded == 0 && misconf == 0 && retryPressure == 0 && routing == 0 && forced == 0 && forcedFailed == 0 && forcedExhausted == 0 && unstable == 0 && art == 0) {
		return nil, nil
	}
	return []Delta{{
		Source:  o.Name(),
		Kind:    "reaper_candidates",
		Summary: fmt.Sprintf("reaper: %d dead agent(s), %d degraded agent(s), %d misconfigured agent(s), %d retry-pressure agent(s), %d routing-pressure agent(s), %d forced-chain probation agent(s), %d forced-chain-failed agent(s), %d forced-chain-exhausted agent(s), %d unstable-routing agent(s), and %d stale artifact(s) — retire, doctor, repair config, calm retry loops, observe forced chains, escalate failed owner routing, escalate exhausted owner routing, stabilize routing, escalate unstable chains, or collect", dead, degraded, misconf, retryPressure, routing, forced, forcedFailed, forcedExhausted, unstable, art),
		Before:  fmt.Sprintf("%d dead / %d degraded / %d misconfigured / %d retry / %d routing / %d forced / %d forced-failed / %d forced-exhausted / %d unstable / %d artifacts", prevDead, prevDegraded, prevMisconf, prevRetry, prevRouting, prevForced, prevForcedFailed, prevForcedExhausted, prevUnstable, prevArt),
		After:   fmt.Sprintf("%d dead / %d degraded / %d misconfigured / %d retry / %d routing / %d forced / %d forced-failed / %d forced-exhausted / %d unstable / %d artifacts", dead, degraded, misconf, retryPressure, routing, forced, forcedFailed, forcedExhausted, unstable, art),
		Hints: map[string]string{
			"severity":                        string(SevLow),
			"dead_agents":                     fmt.Sprintf("%d", dead),
			"degraded_agents":                 fmt.Sprintf("%d", degraded),
			"misconfigured_agents":            fmt.Sprintf("%d", misconf),
			"retry_pressure_agents":           fmt.Sprintf("%d", retryPressure),
			"routing_pressure_agents":         fmt.Sprintf("%d", routing),
			"routing_forced_probation_agents": fmt.Sprintf("%d", forced),
			"routing_forced_failed_agents":    fmt.Sprintf("%d", forcedFailed),
			"routing_forced_exhausted_agents": fmt.Sprintf("%d", forcedExhausted),
			"routing_unstable_agents":         fmt.Sprintf("%d", unstable),
			"stale_artifacts":                 fmt.Sprintf("%d", art),
		},
	}}, nil
}
