// SPDX-License-Identifier: MIT

// Package governor is the per-task routing + budget layer
// (TASKS P1-CONDUIT-01..04; DECISIONS C1-C6).
//
// The Governor implements agent.Provider so the rest of the kernel does
// not need to know it exists; it sits between the agent tool-loop and the
// concrete Provider plugins, choosing which one runs each call, walking a
// fallback chain on error, tracking spend in USD-microcents (DECISIONS C1),
// and enforcing per-day and per-task ceilings.
//
// Routing (M1.b minimum):
//
//  1. If RouteOptions.PreferredProvider is set and registered, try it.
//  2. Otherwise pick the primary (first registered non-fallback provider).
//  3. On a fall-back-able error (anything except context.Canceled /
//     DeadlineExceeded / ErrBudgetExceeded), walk the chain:
//     other non-fallback providers in registration order, then any
//     fallback (IsFallback=true) providers last.
//
// Full subscription→cost→latency policy (DECISIONS C2) lands when the
// model catalog sync (TASKS P1-CONDUIT-04) ships.
package governor

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agezt/agezt/kernel/bus"
)

// DefaultDailyCeilingMicrocents is the per-day spend cap from DECISIONS F3
// ($20/day). Set Config.DailyCeilingMicrocents to 0 to disable the cap;
// negative is treated as 0.
const DefaultDailyCeilingMicrocents int64 = 20 * 100 * 10_000_000 // 20 USD

// Config tunes a Governor.
type Governor struct {
	cfg Config

	mu                sync.Mutex
	spentToday        atomic.Int64     // microcents (global), atomic for hot-path no-lock reads
	spentByTaskToday  map[string]int64 // microcents per task type (M1.zz)
	spentByAgentToday map[string]int64 // microcents per agent slug (M793)
	today             string           // YYYY-MM-DD UTC
	rateWindow        string           // current rate window key (YYYY-MM-DDTHH:MM UTC)
	callsThisWindow   int              // admitted calls in the current rate window

	// ceilingOverride is the operator's runtime-adjusted daily cap (M607),
	// set via SetDailyCeiling from the control plane / Web UI. When
	// ceilingOverridden is true it supersedes cfg.DailyCeilingMicrocents for
	// ALL enforcement and reporting (effectiveCeilingLocked); when false the
	// static config value stands. Guarded by mu — it is read on the spend hot
	// path (budgetExceeded) and in Snapshot, exactly where the config value was
	// read before. 0 is a legal override meaning "unlimited", which is why a
	// separate bool (not a -1 sentinel) distinguishes "no override set".
	ceilingOverride   int64
	ceilingOverridden bool

	// Stable ordering for routing: primary chain + fallback chain. Guarded by
	// chainMu (RWMutex) — Replace rebuilds them on the hot-reload path
	// concurrently with Complete's routeChain/Providers reads.
	chainMu       sync.RWMutex
	primary       []*ProviderInfo // unsorted, insertion-order registry
	sortedPrimary []*ProviderInfo // primary sorted by authModePriority (cached, rebuilt on Replace)
	fallback      []*ProviderInfo

	// taskModelChains is the runtime-mutable per-task-type model fallback chain
	// (M703), seeded from cfg.TaskModelChains and swapped live by
	// SetTaskModelChains (the control plane / Routing UI). Guarded by mu — read
	// as a snapshot in the Complete/CompleteStream chain loop.
	taskModelChains map[string][]string
	// fallbackChains is the runtime-mutable registry of named reusable chains
	// (M963), seeded from cfg.FallbackChains and swapped live by SetFallbackChains.
	// defaultChain names the entry used when a run resolves to no chain. Both are
	// guarded by mu (read under lock; the control plane swaps them on edit).
	fallbackChains map[string][]string
	defaultChain   string

	// breaker is the per-provider circuit breaker (M997): it skips a provider
	// that has failed BreakerThreshold times in a row until a cooldown elapses,
	// so a dead provider doesn't cost latency on every request. nil-safe — a
	// non-positive threshold leaves it disabled.
	breaker *breaker

	// bus is the audit sink, latched atomically so SetBus (which the daemon
	// calls after construction, and WithLimits siblings re-point) never races
	// the lock-free publish read on the hot path.
	bus atomic.Pointer[bus.Bus]

	// usage is a bounded, best-effort per-correlation token index that backs
	// UsageFor — the fast path for the OpenAI-compat `usage` REPORTING field, so a
	// just-completed run's usage is O(1) instead of an O(journal) scan per API
	// response (which a client hammering the API could amplify). It is NOT used for
	// billing or ceiling enforcement (that is spentToday); a miss/eviction simply
	// falls back to the authoritative journal scan in the caller. Guarded by its
	// own lock so it never touches the spend hot path. Two generations (live +
	// previous) bound memory at 2×cap while never wiping a still-accumulating run's
	// partial sum: see indexUsageTokens.
	usageMu   sync.Mutex
	usage     map[string]usageTokens
	usagePrev map[string]usageTokens

	// respCache is the opt-in LLM response cache (M888); nil when disabled
	// (Config.ResponseCacheTTL == 0), which is the default.
	respCache *respCache
}

// usageTokens is the summed input/output token count recorded for one correlation.
type usageTokens struct{ in, out int }

// usageIndexCap bounds each generation of the in-memory usage index. When the
// live generation fills, it rotates to become the previous generation and a fresh
// live map starts (total memory ≤ 2×cap), so an evicted correlation cleanly MISSES
// (→ authoritative journal-scan fallback) instead of being served a partial sum.
// 8192 covers far more than any realistic in-flight set, so the common "usage for
// the run that just finished" lookup is effectively always a hit.
const usageIndexCap = 8192

// New constructs a Governor over cfg. The registry must contain at least
// one provider.
func New(cfg Config) (*Governor, error) {
	if cfg.Registry == nil {
		return nil, errors.New("governor: registry required")
	}
	if len(cfg.Registry.All()) == 0 {
		return nil, errors.New("governor: registry has no providers")
	}
	if cfg.DailyCeilingMicrocents < 0 {
		cfg.DailyCeilingMicrocents = 0
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	g := &Governor{cfg: cfg, spentByTaskToday: map[string]int64{}, spentByAgentToday: map[string]int64{}}
	g.taskModelChains = copyStringSliceMap(cfg.TaskModelChains)
	g.fallbackChains = copyStringSliceMap(cfg.FallbackChains)
	g.defaultChain = cfg.DefaultChain
	if cfg.ResponseCacheTTL > 0 {
		g.respCache = newRespCache(cfg.ResponseCacheTTL, cfg.ResponseCacheSize, cfg.Now) // M888: opt-in
	}
	// Circuit breaker (M997): enabled unless explicitly disabled (negative
	// threshold). 0 → default threshold; default cooldown when unset.
	{
		thr := cfg.BreakerThreshold
		if thr == 0 {
			thr = DefaultBreakerThreshold
		}
		cd := cfg.BreakerCooldown
		if cd <= 0 {
			cd = DefaultBreakerCooldown
		}
		g.breaker = newBreaker(thr, cd, cfg.Now)
	}
	g.bus.Store(cfg.Bus) // may be nil; SetBus latches the real bus later
	for _, p := range cfg.Registry.All() {
		if p.IsFallback {
			g.fallback = append(g.fallback, p)
		} else {
			g.primary = append(g.primary, p)
		}
	}
	g.sortedPrimary = g.sortPrimary() // initial sort (chainMu write not needed: single-threaded init)
	return g, nil
}

// Name implements agent.Provider.
func (g *Governor) Name() string { return "governor" }

// Registry returns the underlying registry for read access. Mutating
// the registry directly bypasses the Governor's chain rebuild — use
// Replace() instead to keep routing in sync.
func (g *Governor) Registry() *Registry { return g.cfg.Registry }

// Replace atomically swaps the named registry entry AND rebuilds the
// internal primary/fallback routing chains. Use this for hot-reload
// paths (M1.r); direct Registry.Replace would update the registry but
// leave the cached chains stale, so the Governor would keep routing
// to the previous provider until daemon restart.
func (g *Governor) Replace(info *ProviderInfo) error {
	if err := g.cfg.Registry.Replace(info); err != nil {
		return err
	}
	// Build FRESH slices rather than truncating + re-appending into the live
	// backing arrays: a concurrent routeChain/Providers that snapshotted the old
	// slice header must keep seeing a consistent (old) backing array, not one
	// being overwritten in place.
	var primary, fallback []*ProviderInfo
	for _, p := range g.cfg.Registry.All() {
		if p.IsFallback {
			fallback = append(fallback, p)
		} else {
			primary = append(primary, p)
		}
	}
	g.chainMu.Lock()
	defer g.chainMu.Unlock()
	g.primary = primary
	g.sortedPrimary = g.sortPrimary() // rebuild cached sorted primary
	g.fallback = fallback
	return nil
}

// ErrBudgetExceeded is returned when the daily ceiling has been spent.
var ErrBudgetExceeded = errors.New("governor: daily budget exceeded")

// ErrTaskBudgetExceeded is returned when a per-task-type cap (M1.zz)
// has been spent for the day. Distinct from ErrBudgetExceeded so
// operators can tell "I've hit my global cap" from "planning calls
// hit their dedicated cap; other task types still have headroom."
// Wraps ErrBudgetExceeded so existing chain-walk logic that treats
// budget-exhaustion as terminal (shouldFallback) catches both.
var ErrTaskBudgetExceeded = fmt.Errorf("%w: task type", ErrBudgetExceeded)

// ErrAgentBudgetExceeded is returned when a named agent's daily spend
// ceiling (roster MaxDailyMc, M793) has been reached for the current
// UTC day. Wraps ErrBudgetExceeded so callers' existing budget checks
// (errors.Is) keep matching.
var ErrAgentBudgetExceeded = fmt.Errorf("%w: agent", ErrBudgetExceeded)

// ErrUnpricedModel is returned (only in StrictPricing mode, M193) when a
// request names a model the governor has no price for. Without strict
// pricing such a model is charged $0 and bypasses the budget; strict mode
// refuses it before any provider call so all billed spend is accounted for.
var ErrUnpricedModel = errors.New("governor: model has no known price (strict pricing)")

// ErrRateLimited is returned when the per-minute call rate has been exceeded.
// Distinct from ErrBudgetExceeded: a rate-limited caller has headroom in its
// daily budget but is calling too fast. It is a transient throttle (the next
// clock-minute admits calls again) raised as a pre-check before any provider is
// tried, so the fallback chain never sees it.
var ErrRateLimited = errors.New("governor: rate limit exceeded")

// ErrModelLacksToolUse is returned (only in StrictModelCapabilities mode)
// when a tools-bearing request targets a model the catalog knows does not
// advertise tool-use. A pre-flight error — no provider is called.
var ErrModelLacksToolUse = errors.New("governor: model does not support tool-use")

// ErrModelUnservable is returned when a model fallback chain entry is skipped
// because no registered provider serves it and every provider declares a
// (non-empty) catalog model list — so dispatching it would only hit a provider
// that 400s on an unrecognised id (M955: the glm-5.1→deepseek misroute). The
// chain walk advances to the next model; this is the last error only when EVERY
// model in the chain was unservable.
var ErrModelUnservable = errors.New("governor: no registered provider serves model")

// ErrNoModelConfigured is returned when a request reaches dispatch with no model
// to send: no per-request/agent chain, no task-type chain, no operator default
// chain, AND an empty req.Model. The daemon ships with NO baked-in default model
// (the owner's "hiçbir default model" rule), so a model must come from
// AGEZT_MODEL, per-task routing, or a fallback chain — otherwise the call cannot
// proceed and the operator is told exactly what to configure.