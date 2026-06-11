// SPDX-License-Identifier: MIT

package pulse

import (
	"context"
	"fmt"
)

// ReaperScanFunc returns the current reaper candidate counts — dead agents and
// stale artifacts. Injected so the observer is testable and pulse doesn't import
// the runtime; the daemon wires k.ReaperScan with its configured cutoffs (M903).
type ReaperScanFunc func() (deadAgents, staleArtifacts int)

// ReaperObserver surfaces stale agents/artifacts on the pulse cadence (#53) —
// detection only; the operator still chooses to retire (graveyard) or collect.
// Transition-based like the disk observer: it fires when the pile of candidates
// first appears or GROWS, not every beat, so a standing backlog doesn't spam the
// briefing. Low severity — this is housekeeping, not an incident.
type ReaperObserver struct {
	scan     ReaperScanFunc
	lastDead int
	lastArt  int
	hasState bool
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
	dead, art := o.scan()

	prevDead, prevArt, known := o.lastDead, o.lastArt, o.hasState
	o.lastDead, o.lastArt, o.hasState = dead, art, true

	if !known {
		return nil, nil // baseline beat: establish state silently
	}
	// Fire only when something NEW went stale (the pile grew), and only when
	// there's actually something to reap now.
	if (dead <= prevDead && art <= prevArt) || (dead == 0 && art == 0) {
		return nil, nil
	}
	return []Delta{{
		Source:  o.Name(),
		Kind:    "reaper_candidates",
		Summary: fmt.Sprintf("reaper: %d dead agent(s) and %d stale artifact(s) — retire or collect", dead, art),
		Before:  fmt.Sprintf("%d agents / %d artifacts", prevDead, prevArt),
		After:   fmt.Sprintf("%d agents / %d artifacts", dead, art),
		Hints:   map[string]string{"severity": string(SevLow)},
	}}, nil
}
