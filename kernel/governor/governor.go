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
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
)

// DefaultDailyCeilingMicrocents is the per-day spend cap from DECISIONS F3
// ($20/day). Set Config.DailyCeilingMicrocents to 0 to disable the cap;
// negative is treated as 0.
const DefaultDailyCeilingMicrocents int64 = 20 * 100 * 10_000_000 // 20 USD

// Config tunes a Governor.
type Config struct {
	// Registry holds the providers.
	Registry *Registry
	// Bus is where budget/routing events are published. If nil, the
	// Governor still works but produces no audit trail (tests only).
	Bus *bus.Bus
	// DailyCeilingMicrocents caps total spend per UTC day. 0 = unlimited.
	DailyCeilingMicrocents int64
	// Now overrides time.Now for tests.
	Now func() time.Time
	// TaskRoutes is the per-task-type routing override (M1.cc). When
	// a CompletionRequest carries a TaskType present in this map, the
	// listed providers are hoisted to the front of the chain (in
	// the listed order, registered-only). Unknown task types fall
	// through to the default subscription-first chain. Nil disables
	// the override entirely. See kernel/governor/routes.go for the
	// TaskRoutes semantics.
	TaskRoutes TaskRoutes
	// TaskModelOverrides replaces CompletionRequest.Model when the
	// req's TaskType matches a configured override (M1.ll). See
	// kernel/governor/routes.go for semantics.
	TaskModelOverrides TaskModelOverrides
	// TaskRouteRequires is the per-task-type *hard* pin (M1.kk).
	// When a TaskType matches, the chain is RESTRICTED to the
	// listed providers (no fallback to others). Use when policy
	// requires it; use TaskRoutes for everything else.
	// Takes precedence over TaskRoutes when both apply to the
	// same task type.
	TaskRouteRequires TaskRouteRequires

	// TaskBudgets caps daily spend per task type (M1.zz). Maps a
	// TaskType to a per-day microcents ceiling. Layered on top of
	// DailyCeilingMicrocents: BOTH must be satisfied for a call to
	// proceed (the request fails fast against whichever fires
	// first). Tasks whose TaskType is not in the map are unaffected
	// by per-task caps — only the global ceiling applies.
	//
	// Use when operators want to bound an expensive class of work
	// (e.g. planning calls capped at $1/day even though the daemon
	// has a $20/day global ceiling), without throttling other
	// classes. Zero or missing entry = no per-task cap.
	TaskBudgets map[string]int64

	// RateLimitPerMin caps the number of completion calls admitted per
	// rolling clock-minute (a fixed window keyed to UTC HH:MM). 0 =
	// unlimited. This is the frequency companion to the spend ceiling:
	// it bounds burst rate (calls/min) independently of cost ($/day), so
	// a per-tenant governor can stop one tenant flooding the shared
	// provider pool even while under its daily budget (M14 quotas).
	RateLimitPerMin int

	// ModelToolCapable, when set, reports whether the given model id
	// advertises tool-use (capable) and whether the catalog knows the
	// model at all (known). Injected by the daemon (backed by the model
	// catalog) so the Governor stays decoupled from kernel/catalog. Used
	// only when StrictModelCapabilities is on. Nil disables the gate.
	ModelToolCapable func(model string) (capable, known bool)

	// StrictModelCapabilities turns the tool-use capability check into a
	// hard pre-flight error (M25). Off by default — the boot advisory
	// (M24) already informs without blocking. When on, a tools-bearing
	// request to a model the catalog KNOWS lacks tool-use is rejected
	// before any provider call; unknown models are never blocked (a
	// catalog-data gap must not break a working setup), and non-tool
	// requests pass regardless.
	StrictModelCapabilities bool

	// StrictPricing turns an unpriced model into a hard pre-flight error
	// (M193). Off by default. When off, a model with no known price
	// (missing from the catalog AND the fallback table) is charged $0, so
	// it silently bypasses the daily/task budget — fail-open. When on, such
	// a request is refused with ErrUnpricedModel BEFORE any provider call,
	// so an operator can guarantee every billed call is accounted for.
	// Known-FREE models (local/mock, present in the table at price 0) are
	// still allowed — only genuinely unknown models are refused. An empty
	// req.Model (provider picks its default) is not gated, since there is
	// no model id to price ahead of the call.
	StrictPricing bool

	// DownRouteToolModels enables capability down-routing (M37): instead of
	// rejecting a tools-bearing request to a known tool-incapable model
	// (M25), the Governor REMAPS req.Model to a tool-capable alternative
	// (via ToolCapableAlternative) and proceeds. Runs before the strict
	// gate, so a successful remap means the gate never fires; if no
	// alternative exists the request falls through to the strict gate's
	// reject (when strict is on) or passes (when it isn't). Off by default.
	DownRouteToolModels bool

	// ToolCapableAlternative, when set, returns a tool-capable substitute
	// model id for a tool-incapable one (and whether one was found). Injected
	// by the daemon, backed by the catalog (same-provider substitute), so the
	// Governor stays decoupled from kernel/catalog. Used only when
	// DownRouteToolModels is on. Nil disables down-routing.
	ToolCapableAlternative func(model string) (alt string, found bool)

	// ModelJSONNative, when set, reports whether the given model id belongs to a
	// provider family with a NATIVE structured-output (JSON mode) switch, and
	// whether the model is known to the catalog at all. Injected by the daemon
	// (backed by catalog.FamilySupportsNativeJSONMode) so the Governor stays
	// decoupled from kernel/catalog. Used to journal capability.degraded when a
	// JSON-mode request lands on a non-native model. Nil disables the check.
	ModelJSONNative func(model string) (native, known bool)
}

// Governor is the per-task routing + budget layer.
type Governor struct {
	cfg Config

	mu               sync.Mutex
	spentToday       int64            // microcents (global)
	spentByTaskToday map[string]int64 // microcents per task type (M1.zz)
	today            string           // YYYY-MM-DD UTC
	rateWindow       string           // current rate window key (YYYY-MM-DDTHH:MM UTC)
	callsThisWindow  int              // admitted calls in the current rate window

	// Stable ordering for routing: primary chain + fallback chain. Guarded by
	// mu — Replace rebuilds them on the hot-reload path concurrently with
	// Complete's routeChain/Providers reads.
	primary  []*ProviderInfo
	fallback []*ProviderInfo

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
	g := &Governor{cfg: cfg, spentByTaskToday: map[string]int64{}}
	g.bus.Store(cfg.Bus) // may be nil; SetBus latches the real bus later
	for _, p := range cfg.Registry.All() {
		if p.IsFallback {
			g.fallback = append(g.fallback, p)
		} else {
			g.primary = append(g.primary, p)
		}
	}
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
	g.mu.Lock()
	defer g.mu.Unlock()
	g.primary = primary
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

// ErrNoProviders is returned when no provider in the chain succeeded.
type ErrNoProviders struct {
	Tried []string
	Last  error
}

func (e *ErrNoProviders) Error() string {
	return fmt.Sprintf("governor: all providers failed (tried %v): %v", e.Tried, e.Last)
}

func (e *ErrNoProviders) Unwrap() error { return e.Last }

// Complete implements agent.Provider. It performs routing + fallback +
// budget accounting; the agent tool-loop sees a single Provider.
//
// Routing hints can be smuggled via the request: not exposed at the
// agent.Provider boundary in M1.b. The future Planner will pass options
// through a richer interface.
func (g *Governor) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	// Per-task-type model override (M1.ll). Mutates the request's
	// Model field before any downstream sees it — providers, audit
	// events, and usage accounting all observe the overridden model
	// id. The original model is recoverable from the operator's
	// config (it's a static mapping) so we don't store it.
	if len(g.cfg.TaskModelOverrides) > 0 && req.TaskType != "" {
		if newModel, ok := g.cfg.TaskModelOverrides[req.TaskType]; ok {
			req.Model = newModel
		}
	}

	// Capability down-routing (M37). Before the strict gate rejects a
	// tools-bearing request to a tool-incapable model, try to REMAP it to a
	// tool-capable alternative. A successful remap means the strict gate
	// below sees a capable model and passes; a miss leaves req.Model
	// unchanged so the gate (if on) still rejects. The remap is journaled so
	// `agt why` shows why the served model differs from the requested one.
	if g.cfg.DownRouteToolModels && g.cfg.ToolCapableAlternative != nil &&
		g.cfg.ModelToolCapable != nil && len(req.Tools) > 0 {
		if capable, known := g.cfg.ModelToolCapable(req.Model); known && !capable {
			if alt, found := g.cfg.ToolCapableAlternative(req.Model); found && alt != req.Model {
				g.publish(event.Spec{
					Subject:       "governor.capability",
					Kind:          event.KindCapabilityRerouted,
					Actor:         "governor",
					CorrelationID: req.CorrelationID,
					Payload: map[string]any{
						"from_model":      req.Model,
						"to_model":        alt,
						"capability":      "tool_call",
						"tools_requested": len(req.Tools),
					},
				})
				req.Model = alt
			}
		}
	}

	// Model capability gate (M25). In strict mode, a tools-bearing request
	// to a model the catalog KNOWS lacks tool-use is rejected pre-flight —
	// converting a confusing deep upstream failure into a clear, journaled
	// error. Checked against the final req.Model (after any task override).
	// Unknown models are never blocked; non-tool requests pass regardless.
	if g.cfg.StrictModelCapabilities && g.cfg.ModelToolCapable != nil && len(req.Tools) > 0 {
		if capable, known := g.cfg.ModelToolCapable(req.Model); known && !capable {
			g.publish(event.Spec{
				Subject:       "governor.capability",
				Kind:          event.KindCapabilityRejected,
				Actor:         "governor",
				CorrelationID: req.CorrelationID,
				Payload: map[string]any{
					"model":           req.Model,
					"capability":      "tool_call",
					"tools_requested": len(req.Tools),
				},
			})
			return nil, fmt.Errorf("%w: model %q (request carries %d tool(s))",
				ErrModelLacksToolUse, req.Model, len(req.Tools))
		}
	}

	// JSON-mode capability DEGRADATION (M381). A JSON-mode request to a provider
	// family with no native structured-output switch is silently honoured by the
	// provider via prompt-instructed JSON — but nothing recorded that the native
	// path was skipped. Unlike tool-use this is NOT fatal and NOT rerouted: the
	// request proceeds on the requested model. Journal it (only when the catalog
	// KNOWS the model is non-native; an unknown model is never flagged) so the
	// degradation is auditable in the run timeline and via `agt why`.
	if req.JSONMode && g.cfg.ModelJSONNative != nil {
		if native, known := g.cfg.ModelJSONNative(req.Model); known && !native {
			g.publish(event.Spec{
				Subject:       "governor.capability",
				Kind:          event.KindCapabilityDegraded,
				Actor:         "governor",
				CorrelationID: req.CorrelationID,
				Payload: map[string]any{
					"model":      req.Model,
					"capability": "json_mode",
					"reason":     "model family has no native JSON mode; relying on prompt-instructed JSON",
				},
			})
		}
	}

	// Rate-limit pre-check (frequency gate, before spend). Admitted calls are
	// counted; a blocked call never reaches a provider or the budget check.
	if admitted, used, limit := g.admitRate(); !admitted {
		g.publish(event.Spec{
			Subject:       "governor.rate",
			Kind:          event.KindRateLimited,
			Actor:         "governor",
			CorrelationID: req.CorrelationID,
			Payload:       map[string]any{"used": used, "limit_per_min": limit},
		})
		return nil, fmt.Errorf("%w (used=%d, limit=%d/min)", ErrRateLimited, used, limit)
	}

	// Budget pre-check (don't even attempt if already past the ceiling). This is
	// a SOFT cap, not a hard reservation: the check and the later recordUsage are
	// separate critical sections with the provider call in between, so N concurrent
	// calls can all observe headroom, all proceed, and together overshoot the
	// ceiling by up to (N-1) calls' worth. Acceptable for a $20/day-class cap with
	// bounded per-call cost; if a hard cap is ever required, reserve estimated cost
	// under the same lock as this check and reconcile the actual after the call.
	// (Reaffirmed as the intended design 2026-06: a hard cap would need a pessimistic
	// pre-call cost estimate and could reject valid calls near the ceiling, which is
	// the worse tradeoff for a bounded-overshoot daily cap.)
	if exceeded, spent, ceiling := g.budgetExceeded(); exceeded {
		g.publish(event.Spec{
			Subject:       "governor.budget",
			Kind:          event.KindBudgetExceeded,
			Actor:         "governor",
			CorrelationID: req.CorrelationID,
			Payload:       map[string]any{"spent_microcents": spent, "ceiling_microcents": ceiling},
		})
		return nil, fmt.Errorf("%w (spent=%d, ceiling=%d microcents)", ErrBudgetExceeded, spent, ceiling)
	}

	// Per-task-type budget pre-check (M1.zz). Only fires when the
	// request carries a TaskType AND that type has a configured cap.
	if req.TaskType != "" {
		if exceeded, spent, cap := g.taskBudgetExceeded(req.TaskType); exceeded {
			g.publish(event.Spec{
				Subject:       "governor.budget",
				Kind:          event.KindBudgetExceeded,
				Actor:         "governor",
				CorrelationID: req.CorrelationID,
				Payload: map[string]any{
					"task_type":          req.TaskType,
					"spent_microcents":   spent,
					"ceiling_microcents": cap,
					"scope":              "task",
				},
			})
			return nil, fmt.Errorf("%w (task=%s, spent=%d, ceiling=%d microcents)",
				ErrTaskBudgetExceeded, req.TaskType, spent, cap)
		}
	}

	// Strict-pricing gate (M193): refuse a model the governor can't price
	// BEFORE spending real money on it. Off by default; when on, an unpriced
	// model (catalog + fallback table miss) would otherwise be charged $0 and
	// silently bypass the budget. Known-free models (in the table at price 0)
	// pass; an empty req.Model (provider default) is not gated.
	if g.cfg.StrictPricing && req.Model != "" && !modelIsPriced(req.Model) {
		g.publish(event.Spec{
			Subject:       "governor.budget",
			Kind:          event.KindBudgetUnpriced,
			Actor:         "governor",
			CorrelationID: req.CorrelationID,
			Payload:       map[string]any{"model": req.Model},
		})
		return nil, fmt.Errorf("%w: %q", ErrUnpricedModel, req.Model)
	}

	chain := g.routeChain(req)
	if len(chain) == 0 {
		return nil, errors.New("governor: no eligible providers")
	}

	// Initial routing decision (first pick). task_type is included
	// so operators using `agt pulse --kind routing.decision` can see
	// which task-type overrides actually fired.
	g.publish(event.Spec{
		Subject:       "governor.route",
		Kind:          event.KindRoutingDecision,
		Actor:         "governor",
		CorrelationID: req.CorrelationID,
		Payload: map[string]any{
			"primary":    chain[0].Name,
			"chain":      providerNames(chain),
			"task_model": req.Model,
			"task_type":  req.TaskType,
		},
	})

	tried := make([]string, 0, len(chain))
	var lastErr error
	for i, p := range chain {
		tried = append(tried, p.Name)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resp, err := p.Provider.Complete(ctx, req)
		if err == nil {
			g.recordUsage(p, req, resp)
			return resp, nil
		}
		lastErr = err
		if !shouldFallback(err) {
			// Don't try further providers when the user cancelled or the
			// budget is exhausted.
			return nil, err
		}
		// Fall back to next in chain.
		if i+1 < len(chain) {
			g.publish(event.Spec{
				Subject:       "governor.fallback",
				Kind:          event.KindProviderFallback,
				Actor:         "governor",
				CorrelationID: req.CorrelationID,
				Payload: map[string]any{
					"failed": p.Name,
					"next":   chain[i+1].Name,
					"reason": err.Error(),
				},
			})
		}
	}
	return nil, &ErrNoProviders{Tried: tried, Last: lastErr}
}

// SpentMicrocents returns the total spend for the current UTC day so far.
// Useful for the future `agt budget` command and for tests.
func (g *Governor) SpentMicrocents() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rolloverIfNeededLocked()
	return g.spentToday
}

// SetBus attaches a bus after construction. The daemon builds the
// Governor before runtime.Open creates the kernel bus, so this lets the
// wiring close the loop without circular-init gymnastics. The pointer is
// latched atomically, so calling it concurrently with an in-flight Complete
// (e.g. re-pointing a WithLimits sibling's bus) is race-free.
func (g *Governor) SetBus(b *bus.Bus) {
	g.bus.Store(b)
}

// DailyCeilingMicrocents returns the configured global daily cap (0 =
// unlimited). Used by the daemon to derive per-tenant ceilings.
func (g *Governor) DailyCeilingMicrocents() int64 {
	return g.cfg.DailyCeilingMicrocents
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
func (g *Governor) Providers() []*ProviderInfo {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]*ProviderInfo, 0, len(g.primary)+len(g.fallback))
	out = append(out, g.primary...)
	out = append(out, g.fallback...)
	return out
}

// ----- internals -----

// routeChain returns the ordered list of providers Complete will try.
// Subscription-first per DECISIONS C2: among primary providers,
// prefer AuthSubscription, then AuthLocal, then AuthAPIKey. Within
// each tier insertion order is preserved (stable sort). Fallback
// providers always come last in registry insertion order regardless
// of auth mode.
//
// Why this order:
//   - AuthSubscription: caller has already paid (Anthropic Pro, ChatGPT
//     Plus, etc.); calling first costs $0 marginal.
//   - AuthLocal: Ollama / local servers; no per-call cost, no rate
//     limit shared with paid keys.
//   - AuthAPIKey: pay-per-token; tried only when the fixed-cost
//     options aren't eligible or failed.
//
// Sorting on every call keeps the behaviour dynamic — when Replace
// promotes a provider from API-key to subscription tier (rare but
// possible after a creds rotation that adds an OAuth refresh token),
// the next Complete picks it up without re-running anything.
func (g *Governor) routeChain(req agent.CompletionRequest) []*ProviderInfo {
	// Snapshot the routing slices under the lock — Replace mutates them on the
	// hot-reload path concurrently with Complete (which calls this unlocked).
	g.mu.Lock()
	primary := make([]*ProviderInfo, len(g.primary))
	copy(primary, g.primary)
	fallback := make([]*ProviderInfo, len(g.fallback))
	copy(fallback, g.fallback)
	g.mu.Unlock()

	slices.SortStableFunc(primary, func(a, b *ProviderInfo) int {
		return authModePriority(a.AuthMode) - authModePriority(b.AuthMode)
	})
	chain := make([]*ProviderInfo, 0, len(primary)+len(fallback))
	chain = append(chain, primary...)
	chain = append(chain, fallback...)
	// Per-task-type HARD pin (M1.kk) takes precedence — when a
	// task type is in TaskRouteRequires, the chain is restricted
	// to the listed providers (no fallback). A nil result from
	// applyTaskRouteRequire is the "all required providers
	// unregistered" sentinel; we let it through unchanged so the
	// Complete loop fails fast with no eligible providers.
	if len(g.cfg.TaskRouteRequires) > 0 && req.TaskType != "" {
		restricted := applyTaskRouteRequire(chain, g.cfg.TaskRouteRequires, req.TaskType)
		// Restricted differs from chain only when the requires entry
		// matched. nil means "matched but nothing registered" — return
		// empty so the caller's "no eligible providers" check fires.
		if restricted == nil {
			return nil
		}
		// applyTaskRouteRequire returns chain unchanged when no
		// requires entry matched the task type; in that case fall
		// through to the soft-preference path below.
		if &restricted[0] != &chain[0] || len(restricted) != len(chain) {
			return restricted
		}
	}
	// Per-task-type soft preference (M1.cc): hoist preferred
	// providers to the front of the chain. Pure reorder — never
	// removes any provider, so the fallback story is preserved.
	if len(g.cfg.TaskRoutes) > 0 && req.TaskType != "" {
		chain = applyTaskRoute(chain, g.cfg.TaskRoutes, req.TaskType)
	}
	// Per-request model routing: when the request names a model, hoist the
	// provider(s) that serve it to the front so a `model` selects its provider
	// (the basis for OpenAI-API model selection across providers). Pure
	// reorder — the fallback chain is preserved if the model's provider fails.
	if req.Model != "" {
		chain = applyModelRoute(chain, req.Model)
	}
	return chain
}

// applyModelRoute hoists providers that serve the given model id to the front
// of the chain, preserving relative order of the rest. A no-op when no provider
// declares the model (the request still runs on the default chain — the named
// provider may still accept the model id even if the catalog didn't list it).
func applyModelRoute(chain []*ProviderInfo, model string) []*ProviderInfo {
	serving := make([]*ProviderInfo, 0, 1)
	rest := make([]*ProviderInfo, 0, len(chain))
	for _, p := range chain {
		if p.Serves(model) {
			serving = append(serving, p)
		} else {
			rest = append(rest, p)
		}
	}
	if len(serving) == 0 {
		return chain
	}
	return append(serving, rest...)
}

// authModePriority maps an AuthMode to a sort key (lower = preferred).
// Unknown auth modes fold to AuthAPIKey's tier (cost-conservative
// default — anything unrecognised is assumed to bill per-call).
func authModePriority(m AuthMode) int {
	switch m {
	case AuthSubscription:
		return 0
	case AuthLocal:
		return 1
	case AuthAPIKey:
		return 2
	default:
		return 2
	}
}

func (g *Governor) recordUsage(p *ProviderInfo, req agent.CompletionRequest, resp *agent.CompletionResponse) {
	model := resp.Usage.Model
	if model == "" {
		model = req.Model
	}
	// Sanitize provider-reported token counts (M191): clamp negatives to
	// 0 so a buggy/hostile usage response can't charge a NEGATIVE cost
	// (which would credit the ledger and eventually disable the daily
	// ceiling). Using the clamped values for BOTH the cost and the audit
	// event keeps the journal honest too. The overflow case is handled by
	// costMicrocents' saturating math.
	inTok := resp.Usage.InputTokens
	if inTok < 0 {
		inTok = 0
	}
	outTok := resp.Usage.OutputTokens
	if outTok < 0 {
		outTok = 0
	}
	// Prompt-cache tokens bill at the cache-read (M289) / cache-write (M291)
	// rates. Clamp to [0, inTok]; cached+write are subsets of the prompt, so a
	// buggy endpoint claiming more must not credit the ledger (costMicrocentsCached
	// re-clamps defensively too).
	cachedTok := resp.Usage.CachedInputTokens
	if cachedTok < 0 {
		cachedTok = 0
	}
	if cachedTok > inTok {
		cachedTok = inTok
	}
	writeTok := resp.Usage.CacheWriteInputTokens
	if writeTok < 0 {
		writeTok = 0
	}
	if cachedTok+writeTok > inTok {
		writeTok = inTok - cachedTok
	}
	cost := costMicrocentsCached(model, inTok, cachedTok, writeTok, outTok)

	g.mu.Lock()
	g.rolloverIfNeededLocked()
	g.spentToday += cost
	if req.TaskType != "" {
		g.spentByTaskToday[req.TaskType] += cost
	}
	spent := g.spentToday
	g.mu.Unlock()

	// Record into the best-effort usage index (reporting fast path) with the SAME
	// token counts that go into budget.consumed below, so a fast-path hit equals
	// the journal sum exactly.
	g.indexUsageTokens(req.CorrelationID, inTok, outTok)

	g.publish(event.Spec{
		Subject: "governor.budget",
		Kind:    event.KindBudgetConsumed,
		Actor:   "governor",
		// Stamp the spending run's correlation (M47) so spend can be
		// attributed per run / per delegation by a journal fold — the same
		// way every other event ties to its run. Empty when the caller set
		// no CorrelationID (e.g. an out-of-run governor call).
		CorrelationID: req.CorrelationID,
		Payload: map[string]any{
			"provider":                 p.Name,
			"model":                    model,
			"input_tokens":             inTok,
			"cached_input_tokens":      cachedTok,
			"cache_write_input_tokens": writeTok,
			"output_tokens":            outTok,
			"cost_microcents":          cost,
			"spent_today_mc":           spent,
			"ceiling_mc":               g.cfg.DailyCeilingMicrocents,
			"correlation_id":           req.CorrelationID,
		},
	})
}

// indexUsageTokens adds one call's token usage to the bounded best-effort
// per-correlation index that backs UsageFor. Summed across a run's calls, exactly
// as the journal fold does. Guarded by its own lock (never the spend hot path).
// Empty correlations are ignored.
//
// Memory is bounded by a two-generation rotation: writes land in the live map;
// when it fills (usageIndexCap), it becomes the previous generation and a fresh
// live map starts (total ≤ 2×cap). The critical property is that a still-running
// correlation's partial sum is NEVER served as authoritative: a write for a corr
// already in the previous generation MIGRATES that accumulated entry into the live
// map before adding, so a hit always reflects the COMPLETE running sum. A corr is
// dropped only when it ages out of BOTH generations untouched — then UsageFor
// cleanly misses and the caller falls back to the authoritative journal scan.
// (The earlier wholesale-drop could leave an in-flight run with a fresh zero entry
// and then serve that PARTIAL sum with ok=true — a silent under-count on the API
// usage field; this rotation removes that hazard.)
func (g *Governor) indexUsageTokens(corr string, in, out int) {
	if corr == "" {
		return
	}
	g.usageMu.Lock()
	defer g.usageMu.Unlock()
	if g.usage == nil {
		g.usage = make(map[string]usageTokens, 64)
	}
	e, live := g.usage[corr]
	if !live {
		// Consolidate any prior-generation accumulation so the live entry holds the
		// complete running sum, never a partial.
		if prev, hadPrev := g.usagePrev[corr]; hadPrev {
			e = prev
			delete(g.usagePrev, corr)
		}
	}
	e.in += in
	e.out += out
	g.usage[corr] = e
	if len(g.usage) >= usageIndexCap {
		g.usagePrev = g.usage
		g.usage = make(map[string]usageTokens, 64)
	}
}

// UsageFor returns the summed input/output tokens recorded for corr from the
// bounded in-memory index, or ok=false when the correlation isn't present (the
// caller then falls back to the authoritative journal scan). Best-effort fast
// path for the API `usage` reporting field; never used for billing or ceilings.
func (g *Governor) UsageFor(corr string) (in, out int, ok bool) {
	g.usageMu.Lock()
	defer g.usageMu.Unlock()
	// Live generation first, then the previous one. Migrate-on-write guarantees a
	// correlation is never split across both, so the first hit is the complete sum.
	if e, hit := g.usage[corr]; hit {
		return e.in, e.out, true
	}
	if e, hit := g.usagePrev[corr]; hit {
		return e.in, e.out, true
	}
	return 0, 0, false
}

// admitRate applies the per-minute fixed-window rate limit. It returns
// (admitted, callsUsedThisWindow, limit). When unlimited (RateLimitPerMin <= 0)
// it always admits without counting. On admission it increments the window
// counter; the window resets when the UTC clock-minute changes.
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

func (g *Governor) budgetExceeded() (bool, int64, int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rolloverIfNeededLocked()
	if g.cfg.DailyCeilingMicrocents <= 0 {
		return false, g.spentToday, 0
	}
	return g.spentToday >= g.cfg.DailyCeilingMicrocents, g.spentToday, g.cfg.DailyCeilingMicrocents
}

// taskBudgetExceeded reports whether the per-task-type ceiling for
// taskType has been spent (M1.zz). Returns (false, 0, 0) when no
// cap is configured for that type — the caller's "if exceeded"
// branch then doesn't fire and the call proceeds.
func (g *Governor) taskBudgetExceeded(taskType string) (bool, int64, int64) {
	if len(g.cfg.TaskBudgets) == 0 {
		return false, 0, 0
	}
	cap, ok := g.cfg.TaskBudgets[taskType]
	if !ok || cap <= 0 {
		return false, 0, 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rolloverIfNeededLocked()
	spent := g.spentByTaskToday[taskType]
	return spent >= cap, spent, cap
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
func (g *Governor) Snapshot() BudgetSnapshot {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rolloverIfNeededLocked()
	snap := BudgetSnapshot{
		UTCDate:           g.today,
		SpentMicrocents:   g.spentToday,
		CeilingMicrocents: g.cfg.DailyCeilingMicrocents,
		StrictPricing:     g.cfg.StrictPricing,
	}
	if len(g.cfg.TaskBudgets) > 0 {
		snap.PerTask = make([]TaskBudgetSnapshot, 0, len(g.cfg.TaskBudgets))
		for taskType, cap := range g.cfg.TaskBudgets {
			snap.PerTask = append(snap.PerTask, TaskBudgetSnapshot{
				TaskType:        taskType,
				SpentMicrocents: g.spentByTaskToday[taskType],
				CapMicrocents:   cap,
			})
		}
	}
	return snap
}

// rolloverIfNeededLocked resets the daily counter at UTC midnight.
// Caller holds g.mu.
func (g *Governor) rolloverIfNeededLocked() {
	today := g.cfg.Now().UTC().Format("2006-01-02")
	if today != g.today {
		g.today = today
		g.spentToday = 0
		// Per-task counters also roll over (M1.zz). Clear the map
		// rather than allocating a fresh one to keep the same
		// underlying memory hot.
		for k := range g.spentByTaskToday {
			delete(g.spentByTaskToday, k)
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
	return true
}

func providerNames(ps []*ProviderInfo) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}
