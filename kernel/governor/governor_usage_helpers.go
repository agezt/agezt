// SPDX-License-Identifier: MIT

// governor_usage_helpers.go: admitRate + SpentByTask/AgentMicrocents +
// rolloverIfNeededLocked + publish + shouldFallback + providerNames split off
// from governor_usage.go during the Day 211 god-file refactor (#143). Public API unchanged.
package governor

import (
	"context"
	"errors"

	"github.com/agezt/agezt/kernel/event"
)

func (g *Governor) admitRate() (bool, int, int) {
	if g.cfg.RateLimitPerMin <= 0 {
		return true, 0, 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	win := g.cfg.Now().UTC().Format("2006-01-02T15:04")
	if win != g.rateWindow {
		g.rateWindow = win
		g.callsThisWindow = 0
	}
	if g.callsThisWindow >= g.cfg.RateLimitPerMin {
		return false, g.callsThisWindow, g.cfg.RateLimitPerMin
	}
	g.callsThisWindow++
	return true, g.callsThisWindow, g.cfg.RateLimitPerMin
}

// SpentByTaskMicrocents returns the current-day spend for taskType
// in microcents. Useful for the operator-facing `agt budget` view
// and for tests. Returns 0 for unknown / unspent task types.
func (g *Governor) SpentByTaskMicrocents(taskType string) int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rolloverIfNeededLocked()
	return g.spentByTaskToday[taskType]
}

// SpentByAgentMicrocents returns the current-day spend attributed to a named
// agent (M793) in microcents. 0 for unknown / unspent agents.
func (g *Governor) SpentByAgentMicrocents(slug string) int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rolloverIfNeededLocked()
	return g.spentByAgentToday[slug]
}

// TaskBudgetSnapshot is one row of the per-task budget view
// returned by Snapshot. SpentMicrocents is the current spend;
// CeilingMicrocents is the configured cap (always > 0 since
// zero-cap entries are filtered at parse time).
type TaskBudgetSnapshot struct {
	TaskType        string
	SpentMicrocents int64
	CapMicrocents   int64
}

// BudgetSnapshot is the read-only view powering `agt budget`
// (and any future operator-facing budget UI). All counters are
// for the current UTC day; UTCDate names that day so callers
// can render "as of 2026-05-29" without a separate field.
type BudgetSnapshot struct {
	UTCDate           string
	SpentMicrocents   int64
	CeilingMicrocents int64
	PerTask           []TaskBudgetSnapshot
	// StrictPricing reflects whether unpriced models are refused (M193/M194)
	// rather than silently charged $0 — part of the operator's spend-protection
	// posture surfaced by `agt budget`.
	StrictPricing bool
}

// Snapshot returns a point-in-time copy of the governor's budget
// state. Holds the mutex for the duration; callers should treat
// the returned struct as immutable. Per-task entries are returned
// for every type with a configured cap (NOT only ones with spend
// > 0) so the operator sees "I configured a cap but nothing has
// hit it" as a separate state from "no cap configured."
func (g *Governor) rolloverIfNeededLocked() {
	today := g.cfg.Now().UTC().Format("2006-01-02")
	if today != g.today {
		g.today = today
		g.spentToday.Store(0)
		// Per-task counters also roll over (M1.zz). Clear the map
		// rather than allocating a fresh one to keep the same
		// underlying memory hot.
		for k := range g.spentByTaskToday {
			delete(g.spentByTaskToday, k)
		}
		// Per-agent counters too (M793).
		for k := range g.spentByAgentToday {
			delete(g.spentByAgentToday, k)
		}
	}
}

func (g *Governor) publish(spec event.Spec) {
	b := g.bus.Load()
	if b == nil {
		return
	}
	_, _ = b.Publish(spec)
}

// shouldFallback decides whether to walk further down the provider chain.
// Cancellation and the budget error are terminal; everything else (rate
// limits, transient HTTP errors, parse errors) is fall-back-able in M1.b.
// A richer classification (DECISIONS C3 borderline-escalation, transient
// vs. terminal) lands with the catalog sync.
func shouldFallback(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, ErrBudgetExceeded) {
		return false
	}
	if errors.Is(err, ErrStreamInterrupted) {
		// Output already reached the consumer (M882): a retry/fallback would
		// duplicate the stream, so the failure is terminal.
		return false
	}
	return true
}

func providerNames(ps []*ProviderInfo) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}
