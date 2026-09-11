// SPDX-License-Identifier: MIT

package governor

// Governor ceiling/budget accessors: SpentMicrocents + SetBus +
// DailyCeilingMicrocents + effectiveCeilingLocked + SetDailyCeiling +
// StrictPricingEnabled + WithDailyCeiling + WithLimits. Carved out of
// governor.go during the Day 29 god file split #2.

import (
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
)

func (g *Governor) SpentMicrocents() int64 {
	g.mu.Lock()
	g.rolloverIfNeededLocked()
	g.mu.Unlock()
	return g.spentToday.Load()
}

// SetBus attaches a bus after construction. The daemon builds the
// Governor before runtime.Open creates the kernel bus, so this lets the
// wiring close the loop without circular-init gymnastics. The pointer is
// latched atomically, so calling it concurrently with an in-flight Complete
// (e.g. re-pointing a WithLimits sibling's bus) is race-free.
func (g *Governor) SetBus(b *bus.Bus) {
	g.bus.Store(b)
}

// DailyCeilingMicrocents returns the EFFECTIVE global daily cap (0 =
// unlimited) — the runtime override if an operator has set one, else the
// configured value. Used by the daemon to derive per-tenant ceilings and by
// `agt budget`. Takes the lock (not hot-path) so it observes a live override.
func (g *Governor) DailyCeilingMicrocents() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.effectiveCeilingLocked()
}

// effectiveCeilingLocked returns the daily ceiling enforcement and reporting
// should use: the operator's runtime override when set (M607), otherwise the
// static config value. Caller holds g.mu.
func (g *Governor) effectiveCeilingLocked() int64 {
	if g.ceilingOverridden {
		return g.ceilingOverride
	}
	return g.cfg.DailyCeilingMicrocents
}

// SetDailyCeiling adjusts the global daily spend cap at runtime (M607) — the
// operator's "ayarla" knob, reachable from the control plane and the Web UI
// cockpit. A negative value is clamped to 0 (unlimited). The new ceiling takes
// effect immediately for the next budget pre-check; spend already booked today
// is unaffected, so lowering the cap below today's spend simply blocks further
// calls until UTC rollover. Returns the effective ceiling now in force and
// emits a budget.ceiling_set audit event. Per-tenant sibling governors created
// by WithLimits keep their own (separately settable) ceilings.
func (g *Governor) SetDailyCeiling(microcents int64) int64 {
	if microcents < 0 {
		microcents = 0
	}
	g.mu.Lock()
	prev := g.effectiveCeilingLocked()
	g.ceilingOverride = microcents
	g.ceilingOverridden = true
	spent := g.spentToday.Load()
	g.mu.Unlock()

	g.publish(event.Spec{
		Subject: "governor.budget",
		Kind:    event.KindBudgetCeilingSet,
		Actor:   "operator",
		Payload: map[string]any{
			"ceiling_mc":      microcents,
			"prev_ceiling_mc": prev,
			"spent_today_mc":  spent,
		},
	})
	return microcents
}

// StrictPricingEnabled reports whether unpriced models are refused (M195).
// cfg is immutable after New, so this is a lock-free read — used by the
// dry-run to predict whether a run on an unpriced model would be refused.
func (g *Governor) StrictPricingEnabled() bool {
	return g.cfg.StrictPricing
}

// WithDailyCeiling returns a sibling Governor that shares this one's
// registry, routing config, and task budgets but keeps an INDEPENDENT
// daily-spend ledger and its own global ceiling. The bus is inherited
// (the caller typically re-points it with SetBus to the sibling's own
// kernel bus before first use).
//
// This is the multi-tenant quota seam (M14): each tenant gets its own
// Governor so one tenant's spend — and its exhaustion of the daily cap —
// can never block another's, while the underlying provider pool and
// credentials stay shared. ceiling is in microcents (0 = unlimited).
func (g *Governor) WithDailyCeiling(ceiling int64) (*Governor, error) {
	return g.WithLimits(ceiling, g.cfg.RateLimitPerMin)
}

// WithLimits is WithDailyCeiling plus an independent per-minute rate cap: the
// sibling shares the parent's provider pool and routing but gets its own spend
// ledger AND its own rate-window counter, with both limits overridden. ratePerMin
// <= 0 means no rate cap. This is the full per-tenant quota seam (M14): cost and
// frequency bounded per tenant, the pool shared.
func (g *Governor) WithLimits(ceiling int64, ratePerMin int) (*Governor, error) {
	ncfg := g.cfg // copy: shares Registry/Bus pointers, copies scalars/maps refs
	ncfg.DailyCeilingMicrocents = ceiling
	ncfg.RateLimitPerMin = ratePerMin
	return New(ncfg)
}

// Providers returns a snapshot of the routing chain (primary first,
// fallback last). Used by the daemon banner.
