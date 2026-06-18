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
	"math/rand/v2"
	"slices"
	"strings"
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
	// TaskModelChains is the per-task-type ORDERED model fallback chain
	// (M703): task type → [primary model, fallback model, …]. When set for a
	// task, the governor tries each model in turn (each routing to its serving
	// provider), falling back model→model — superseding TaskModelOverrides for
	// that task. Seeds the runtime-mutable chains (SetTaskModelChains). See
	// kernel/governor/routes.go.
	TaskModelChains TaskModelChains
	// FallbackChains is the registry of NAMED, reusable model fallback chains
	// (M963): chain name → [primary model, fallback model, …]. Anywhere a model
	// id is expected (an agent's model, a task chain entry, a per-run model, the
	// default) the token "@<name>" references a named chain and is expanded into
	// that chain's models at completion time — so one chain edited in one place
	// propagates everywhere it's referenced. Seeds the runtime-mutable registry
	// (SetFallbackChains).
	FallbackChains map[string][]string
	// DefaultChain, when set, names the FallbackChains entry used for any run that
	// resolves to no chain of its own (no agent chain, no task chain, no explicit
	// model) — so even a bare run gets the operator's default fallback ladder.
	DefaultChain string
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

	// ModelStrictToolArgsNative, when set, reports whether the given model id can
	// enforce declared tool-argument schemas at generation/sampler time, and
	// whether the model is known to the catalog at all. When a tools-bearing
	// request lands on a known model without this capability, the Governor
	// journals a capability.degraded event and lets the kernel boundary validator
	// remain the enforcement fallback. Nil disables the check.
	ModelStrictToolArgsNative func(model string) (native, known bool)

	// ResponseCacheTTL enables the OPT-IN LLM response cache (M888): an
	// IDENTICAL CompletionRequest within the TTL is served from memory — no
	// provider call, no tokens, no spend. 0 (the default) disables caching
	// entirely, because an LLM is not a pure function and chat "regenerate"
	// wants a fresh sample. Enable for machine-driven workloads whose repeat
	// calls are deterministic re-asks (retried workflow steps, re-fired
	// schedules over unchanged input, parallel sub-agents asking the same
	// question). See kernel/governor/cache.go.
	ResponseCacheTTL time.Duration
	// ResponseCacheSize bounds the cache's LRU entry count. 0 →
	// DefaultResponseCacheSize. Only meaningful with ResponseCacheTTL > 0.
	ResponseCacheSize int

	// ProviderRetries is how many times ONE provider is retried in place (with
	// exponential backoff) on a TRANSIENT error — rate limit, 5xx, network blip
	// — before the chain falls back to the next provider (M882). A transient
	// 429 on the primary used to cost an immediate downgrade to a fallback
	// provider/model; a short retry usually keeps the call on the best route.
	// Non-transient errors (auth, invalid request) still fall back immediately.
	// 0 → DefaultProviderRetries; negative → no in-place retries (the
	// historical immediate-fallback behaviour).
	ProviderRetries int
	// RetryBaseDelay is the first backoff delay; each subsequent retry doubles
	// it, plus up to 25% jitter so synchronized callers don't stampede a
	// recovering upstream. 0 → DefaultRetryBaseDelay.
	RetryBaseDelay time.Duration
}

// DefaultProviderRetries is the default number of in-place retries per
// provider on a transient error (M882): 2 retries → 3 attempts total,
// ~0.5s + ~1s of backoff — enough to ride out a rate-limit window without
// meaningfully delaying a genuine outage's fallback.
const DefaultProviderRetries = 2

// DefaultRetryBaseDelay is the first in-place retry's backoff (M882).
const DefaultRetryBaseDelay = 500 * time.Millisecond

// Governor is the per-task routing + budget layer.
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
type ErrNoModelConfigured struct {
	TaskType string
}

func (e *ErrNoModelConfigured) Error() string {
	if e != nil && strings.TrimSpace(e.TaskType) != "" {
		return fmt.Sprintf("governor: no model configured for task %q — set AGEZT_MODEL, a per-task routing model, or a fallback chain", e.TaskType)
	}
	return "governor: no model configured — set AGEZT_MODEL, a per-task routing model, or a fallback chain"
}

// ErrNoProviders is returned when no provider in the chain succeeded.
type ErrNoProviders struct {
	Tried []string
	Last  error
}

func (e *ErrNoProviders) Error() string {
	return fmt.Sprintf("governor: all providers failed (tried %v): %v", e.Tried, e.Last)
}

func (e *ErrNoProviders) Unwrap() error { return e.Last }

// preflightAndRoute runs every pre-call gate (task/down-route model remap,
// capability + strict-pricing gates, rate-limit and budget pre-checks) against
// req — mutating it in place where a gate remaps the model — then resolves and
// announces the provider chain. Shared by Complete and CompleteStream so the
// governed call is byte-for-byte identical whether or not the response streams.
// Returns the routed chain, or a non-nil error if any gate refuses the call.
//
// Routing hints can be smuggled via the request: not exposed at the
// agent.Provider boundary in M1.b. The future Planner will pass options
// through a richer interface.
func (g *Governor) preflightAndRoute(req *agent.CompletionRequest) ([]*ProviderInfo, error) {
	// Per-task-type model override (M1.ll). Mutates the request's
	// Model field before any downstream sees it — providers, audit
	// events, and usage accounting all observe the overridden model
	// id. The original model is recoverable from the operator's
	// config (it's a static mapping) so we don't store it.
	//
	// A configured model CHAIN (M703) supersedes the single override for that
	// task type: completeChained has already set req.Model to the chain's
	// current model, so we must NOT clobber it here.
	if len(g.cfg.TaskModelOverrides) > 0 && req.TaskType != "" {
		g.mu.Lock()
		_, hasChain := g.taskModelChains[req.TaskType]
		g.mu.Unlock()
		if !hasChain {
			if newModel, ok := g.cfg.TaskModelOverrides[req.TaskType]; ok {
				req.Model = newModel
			}
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

	// Strict tool-argument capability DEGRADATION (CH-02). Tool schemas are always
	// validated at the kernel boundary, but some provider/model pairs can also
	// prevent malformed arguments at generation time. If the catalog knows the
	// final model lacks that native constrained path, record the fallback so the
	// run timeline explains why invalid tool calls may be detected after sampling
	// instead of made impossible by the sampler.
	if len(req.Tools) > 0 && g.cfg.ModelStrictToolArgsNative != nil {
		if native, known := g.cfg.ModelStrictToolArgsNative(req.Model); known && !native {
			g.publish(event.Spec{
				Subject:       "governor.capability",
				Kind:          event.KindCapabilityDegraded,
				Actor:         "governor",
				CorrelationID: req.CorrelationID,
				Payload: map[string]any{
					"model":           req.Model,
					"capability":      "strict_tool_args",
					"tools_requested": len(req.Tools),
					"reason":          "model does not advertise schema-constrained tool arguments; relying on kernel validation fallback",
				},
			})
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

	// Per-agent daily budget pre-check (M793): a named agent's requests carry
	// its slug and daily ceiling (roster MaxDailyMc); once today's identity
	// ledger reaches the ceiling, further completions are refused — the
	// per-day analogue of the profile's per-run cost cap.
	if req.Agent != "" && req.AgentDailyCeilingMc > 0 {
		// Read AND compare under the same lock so the decision reflects a
		// consistent snapshot (no released-lock gap). Note: like the global and
		// per-task caps this remains a pre-check — the actual charge lands after
		// the completion returns, so requests already in flight for one agent can
		// still collectively overshoot by the in-flight set. Hard per-request
		// reservation would require up-front cost estimation (out of scope here).
		g.mu.Lock()
		g.rolloverIfNeededLocked()
		agentSpent := g.spentByAgentToday[req.Agent]
		exceeded := agentSpent >= req.AgentDailyCeilingMc
		g.mu.Unlock()
		if exceeded {
			g.publish(event.Spec{
				Subject:       "governor.budget",
				Kind:          event.KindBudgetExceeded,
				Actor:         "governor",
				CorrelationID: req.CorrelationID,
				Payload: map[string]any{
					"agent":              req.Agent,
					"spent_microcents":   agentSpent,
					"ceiling_microcents": req.AgentDailyCeilingMc,
					"scope":              "agent",
				},
			})
			return nil, fmt.Errorf("%w (agent=%s, spent=%d, ceiling=%d microcents)",
				ErrAgentBudgetExceeded, req.Agent, agentSpent, req.AgentDailyCeilingMc)
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

	chain := g.routeChain(*req)
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

	return chain, nil
}

// runChain executes req against the routed provider chain, recording usage on
// the first success and falling back on retryable errors. callOne performs the
// actual provider call for one chain entry — Complete passes the non-streaming
// call, CompleteStream the streaming one — so routing, fallback and usage
// accounting are identical for both.
func (g *Governor) runChain(ctx context.Context, req agent.CompletionRequest, chain []*ProviderInfo, callOne func(*ProviderInfo) (*agent.CompletionResponse, error)) (*agent.CompletionResponse, error) {
	tried := make([]string, 0, len(chain))
	var lastErr error
	for i, p := range chain {
		tried = append(tried, p.Name)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resp, err := g.callWithRetry(ctx, req, p, callOne)
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

// callWithRetry invokes one chain entry, retrying IN PLACE with exponential
// backoff + jitter on transient errors (M882) before the caller falls back to
// the next provider. Terminal errors (cancel, budget, stream-interrupted) and
// non-transient provider errors (auth, invalid request) return immediately —
// retrying those wastes time at best and duplicates output at worst.
func (g *Governor) callWithRetry(ctx context.Context, req agent.CompletionRequest, p *ProviderInfo, callOne func(*ProviderInfo) (*agent.CompletionResponse, error)) (*agent.CompletionResponse, error) {
	retries := g.cfg.ProviderRetries
	if retries == 0 {
		retries = DefaultProviderRetries
	}
	if retries < 0 {
		retries = 0
	}
	base := g.cfg.RetryBaseDelay
	if base <= 0 {
		base = DefaultRetryBaseDelay
	}
	for attempt := 0; ; attempt++ {
		resp, err := callOne(p)
		if err == nil {
			return resp, nil
		}
		if attempt >= retries || !shouldFallback(err) || !isTransient(err) {
			return nil, err
		}
		delay := base << attempt // 0.5s, 1s, 2s, …
		if delay >= 4 {
			delay += time.Duration(rand.Int64N(int64(delay) / 4)) // +0–25% jitter
		}
		g.publish(event.Spec{
			Subject:       "governor.retry",
			Kind:          event.KindProviderRetry,
			Actor:         "governor",
			CorrelationID: req.CorrelationID,
			Payload: map[string]any{
				"provider": p.Name,
				"attempt":  attempt + 1,
				"of":       retries,
				"delay_ms": delay.Milliseconds(),
				"reason":   err.Error(),
			},
		})
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		case <-t.C:
		}
	}
}

// isTransient reports whether a provider error looks like a passing condition
// worth retrying on the SAME provider: rate limiting, upstream overload/5xx,
// or a network blip. Provider adapters surface upstream failures as wrapped
// text errors (no structured status crosses the plugin boundary), so this is
// a deliberately conservative substring match — an unrecognised error falls
// back to the next provider immediately, the historical behaviour.
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"429", "rate limit", "rate_limit", "too many requests",
		"overloaded", "overloaded_error", "529",
		"500", "502", "503", "504",
		"internal server error", "bad gateway", "service unavailable", "gateway timeout",
		"timeout", "timed out", "deadline exceeded",
		"connection refused", "connection reset", "broken pipe",
		"unexpected eof", "eof",
		"temporarily unavailable", "try again",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// ErrStreamInterrupted marks a streaming call that failed AFTER chunks were
// already delivered to the consumer (M882). Retrying or falling back would
// replay the stream from the start and duplicate everything the user already
// saw, so the governor treats it as terminal and surfaces the upstream error.
var ErrStreamInterrupted = errors.New("governor: stream interrupted after output started")

// Complete implements agent.Provider: the full pre-call gate set, routing and
// fallback over the non-streaming provider call. The agent tool-loop sees a
// single Provider.
func (g *Governor) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	// Opt-in response cache (M888): an exact repeat within the TTL is served
	// from memory — checked BEFORE preflight so a hit consumes neither a
	// rate-window slot nor budget (it costs the upstream nothing).
	var key string
	if g.respCache != nil {
		key = cacheKey(req)
		if resp, ok := g.respCache.get(key); ok {
			g.publish(event.Spec{
				Subject:       "governor.cache",
				Kind:          event.KindRoutingDecision,
				Actor:         "governor",
				CorrelationID: req.CorrelationID,
				Payload:       map[string]any{"cache": "hit", "task_model": req.Model, "task_type": req.TaskType},
			})
			return resp, nil
		}
	}
	resp, err := g.completeChained(req, func(r agent.CompletionRequest) (*agent.CompletionResponse, error) {
		chain, err := g.preflightAndRoute(&r)
		if err != nil {
			return nil, err
		}
		return g.runChain(ctx, r, chain, func(p *ProviderInfo) (*agent.CompletionResponse, error) {
			return p.Provider.Complete(ctx, r)
		})
	})
	if err == nil && resp != nil && g.respCache != nil {
		g.respCache.put(key, *resp)
	}
	return resp, err
}

// CompleteStream implements agent.StreamingProvider so the GOVERNED provider
// streams token/reasoning deltas to the loop (M1.q.y) instead of collapsing to
// a single response — through the exact same routing, fallback, budget and usage
// path as Complete. Before this, the governor (which every real run goes
// through) only satisfied agent.Provider, so the loop's streaming branch never
// engaged and the Web UI Chat never streamed live with a real provider. A chain
// entry that isn't itself streaming-capable (e.g. the offline mock fallback) is
// called non-streaming and simply yields no deltas; the assembled response still
// flows back unchanged.
func (g *Governor) CompleteStream(ctx context.Context, req agent.CompletionRequest, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	// Track whether any chunk has reached the consumer (M882). Until the first
	// chunk, a streaming failure retries / falls back exactly like Complete —
	// full parity. AFTER output has started flowing, a retry or fallback would
	// replay the stream from the start and duplicate everything already shown
	// (and journaled as llm.token events), so the failure becomes terminal.
	// onChunk is called sequentially within one attempt and attempts are
	// sequential, so a plain bool needs no lock.
	emitted := false
	wrapped := func(c agent.Chunk) error {
		if !c.IsEmpty() || c.ReasoningDelta != "" {
			emitted = true
		}
		return onChunk(c)
	}
	return g.completeChained(req, func(r agent.CompletionRequest) (*agent.CompletionResponse, error) {
		chain, err := g.preflightAndRoute(&r)
		if err != nil {
			return nil, err
		}
		return g.runChain(ctx, r, chain, func(p *ProviderInfo) (*agent.CompletionResponse, error) {
			if sp, ok := p.Provider.(agent.StreamingProvider); ok {
				resp, err := sp.CompleteStream(ctx, r, wrapped)
				if err != nil && emitted {
					return nil, fmt.Errorf("%w: %w", ErrStreamInterrupted, err)
				}
				return resp, err
			}
			return p.Provider.Complete(ctx, r)
		})
	})
}

// completeChained runs req across its task type's model fallback chain (M703),
// if any: it tries each model in order via runOne (the full preflight + provider
// chain), and on a fallback-eligible failure of one model's WHOLE attempt it
// moves to the NEXT MODEL. With no chain configured it calls runOne once with req
// unchanged — byte-for-byte the pre-M703 single-model path. Terminal errors
// (context cancel, budget exhaustion) stop the walk immediately.
//
// Note: each model re-runs preflight (rate/budget pre-checks), so a request that
// actually falls back consumes one rate-window slot per model TRIED — acceptable
// for a soft burst cap, and only on the rare provider-failure path.
func (g *Governor) completeChained(req agent.CompletionRequest, runOne func(agent.CompletionRequest) (*agent.CompletionResponse, error)) (*agent.CompletionResponse, error) {
	// A per-request chain (M787 — a named agent's own fallbacks) WINS over
	// the task type's configured chain: the more specific identity beats the
	// broader category. The fallback events stay distinguishable via scope.
	models := req.ModelChain
	scope := "agent-chain"
	if len(models) == 0 {
		models = g.modelChainFor(req.TaskType)
		scope = "model-chain"
	}
	if len(models) == 0 {
		// No agent/task/explicit chain — fall to the operator's default named
		// chain so even a bare run gets the configured fallback ladder (M963).
		if def := g.defaultChainModels(); len(def) > 0 {
			models = def
			scope = "default-chain"
		}
	}
	// Expand any "@<name>" references into the named chain's models (M963). One
	// pass covers every source (agent, task, default) since they all flow here.
	models = g.expandChains(models)
	if len(models) == 0 {
		// No chain resolved a model. With the daemon's default-model removed,
		// an empty req.Model has nowhere to come from — refuse with an
		// actionable error instead of dispatching a blank model to the provider
		// (which would 400 with an opaque message).
		if strings.TrimSpace(req.Model) == "" {
			return nil, &ErrNoModelConfigured{TaskType: req.TaskType}
		}
		return runOne(req)
	}
	var lastErr error
	for i, m := range models {
		// Skip a chain model that NO registered provider can serve (M955).
		// Without this, applyModelRoute leaves the default chain in place and
		// the model id is dispatched to the primary provider, which 400s on an
		// id it doesn't recognise — one failed call PER provider in the chain,
		// a fallback storm — before the walk finally reaches the next model.
		// Skipping straight to the next model produces a real answer with zero
		// doomed calls. Guarded to "definitively unservable" (every provider
		// declares a model list and none include m) so an unknown-coverage
		// provider (empty Models, e.g. the mock/echo fallback) still gets the
		// benefit of the doubt — its presence preserves the legacy fall-through.
		if g.modelKnownUnservable(m) {
			lastErr = fmt.Errorf("%w: %q", ErrModelUnservable, m)
			if i+1 < len(models) {
				g.publish(event.Spec{
					Subject:       "governor.fallback",
					Kind:          event.KindProviderFallback,
					Actor:         "governor",
					CorrelationID: req.CorrelationID,
					Payload: map[string]any{
						"failed_model": m,
						"next_model":   models[i+1],
						"reason":       "no registered provider serves this model",
						"scope":        scope,
						"task_type":    req.TaskType,
						"skipped":      true,
					},
				})
			}
			continue
		}
		attempt := req
		attempt.Model = m
		resp, err := runOne(attempt)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !shouldFallback(err) {
			return nil, err
		}
		if i+1 < len(models) {
			g.publish(event.Spec{
				Subject:       "governor.fallback",
				Kind:          event.KindProviderFallback,
				Actor:         "governor",
				CorrelationID: req.CorrelationID,
				Payload: map[string]any{
					"failed_model": m,
					"next_model":   models[i+1],
					"reason":       err.Error(),
					"scope":        scope,
					"task_type":    req.TaskType,
				},
			})
		}
	}
	return nil, lastErr
}

// modelChainFor returns a copy of the configured model fallback chain for the
// task type, or nil if none. Read under mu (SetTaskModelChains may swap it).
func (g *Governor) modelChainFor(taskType string) []string {
	if taskType == "" {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	src := g.taskModelChains[taskType]
	if len(src) == 0 {
		return nil
	}
	return slices.Clone(src)
}

// modelKnownUnservable reports whether the registered providers DEFINITIVELY
// cannot serve model: no registered provider lists it AND every registered
// provider declares a non-empty catalog model list. An empty Models list means
// "unknown coverage" (the comment on ProviderInfo.Models) — such a provider may
// accept an unlisted id, so its presence makes the verdict false (don't skip).
// Mirrors the routeChain snapshot discipline: read the chain slices under
// chainMu so a concurrent Replace (hot reload) can't race the scan.
func (g *Governor) modelKnownUnservable(model string) bool {
	if model == "" {
		return false
	}
	g.chainMu.RLock()
	defer g.chainMu.RUnlock()
	for _, p := range g.sortedPrimary {
		if p.Serves(model) || len(p.Models) == 0 {
			return false
		}
	}
	for _, p := range g.fallback {
		if p.Serves(model) || len(p.Models) == 0 {
			return false
		}
	}
	// Guard: with no providers at all (impossible post-New, but cheap) treat as
	// servable so we never skip every model on an empty registry.
	if len(g.sortedPrimary) == 0 && len(g.fallback) == 0 {
		return false
	}
	return true
}

// SetTaskModelChains atomically replaces the per-task-type model fallback chains
// (M703) — the hot-reload path from the control plane / Routing UI. A nil/empty
// map clears all chains (routing reverts to single-model + provider fallback).
func (g *Governor) SetTaskModelChains(chains map[string][]string) {
	g.mu.Lock()
	g.taskModelChains = copyStringSliceMap(chains)
	g.mu.Unlock()
}

// TaskModelChainsView returns a copy of the effective per-task-type model chains.
func (g *Governor) TaskModelChainsView() map[string][]string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return copyStringSliceMap(g.taskModelChains)
}

// ChainPrefix marks a model-id slot as a reference to a NAMED fallback chain
// (M963): "@fast" means "expand to the models of the chain called fast". Model
// ids never start with "@", so the token is unambiguous and works inside the
// comma/semicolon task-chain syntax too.
const ChainPrefix = "@"

// SetFallbackChains atomically replaces the named-chain registry and the default
// chain name (M963) — the hot-reload path from the control plane / Chains UI.
func (g *Governor) SetFallbackChains(chains map[string][]string, defaultChain string) {
	g.mu.Lock()
	g.fallbackChains = copyStringSliceMap(chains)
	g.defaultChain = defaultChain
	g.mu.Unlock()
}

// FallbackChainsView returns a copy of the named-chain registry and the default
// chain name.
func (g *Governor) FallbackChainsView() (map[string][]string, string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return copyStringSliceMap(g.fallbackChains), g.defaultChain
}

// defaultChainModels returns the models of the configured default chain, or nil.
func (g *Governor) defaultChainModels() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.defaultChain == "" {
		return nil
	}
	if src := g.fallbackChains[g.defaultChain]; len(src) > 0 {
		return slices.Clone(src)
	}
	return nil
}

// expandChains replaces every "@<name>" reference in a model list with the
// models of that named chain (M963), flattening and de-duplicating while
// preserving order. Unknown chains are dropped (a deleted chain must not crash a
// run). Non-reference ids pass through unchanged. One level only — chains hold
// real model ids, validated on save, so there is no recursion to worry about.
func (g *Governor) expandChains(models []string) []string {
	if len(models) == 0 {
		return models
	}
	g.mu.Lock()
	reg := g.fallbackChains
	out := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	add := func(m string) {
		if m == "" {
			return
		}
		if _, dup := seen[m]; dup {
			return
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	for _, m := range models {
		if name, ok := strings.CutPrefix(m, ChainPrefix); ok {
			for _, cm := range reg[name] {
				add(cm)
			}
			continue
		}
		add(m)
	}
	g.mu.Unlock()
	return out
}

// SpentMicrocents returns the total spend for the current UTC day so far.
// Useful for the future `agt budget` command and for tests.
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
func (g *Governor) Providers() []*ProviderInfo {
	g.chainMu.RLock()
	defer g.chainMu.RUnlock()
	out := make([]*ProviderInfo, 0, len(g.primary)+len(g.fallback))
	out = append(out, g.primary...)
	out = append(out, g.fallback...)
	return out
}

// ----- internals -----

// sortPrimary returns a sorted copy of g.primary by authModePriority.
// Caller holds g.mu.
func (g *Governor) sortPrimary() []*ProviderInfo {
	sorted := make([]*ProviderInfo, len(g.primary))
	copy(sorted, g.primary)
	slices.SortStableFunc(sorted, func(a, b *ProviderInfo) int {
		return authModePriority(a.AuthMode) - authModePriority(b.AuthMode)
	})
	return sorted
}

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
// The primary sort is cached in sortedPrimary (rebuilt on Replace) to avoid
// O(n log n) sort on every Complete call. Replace also updates the cache
// when a provider's AuthMode changes (e.g. creds rotation adds OAuth).
func (g *Governor) routeChain(req agent.CompletionRequest) []*ProviderInfo {
	// Snapshot the routing slices under the chain lock — Replace mutates them on
	// the hot-reload path concurrently with Complete (which calls this unlocked).
	g.chainMu.RLock()
	primary := make([]*ProviderInfo, len(g.sortedPrimary))
	copy(primary, g.sortedPrimary)
	fallback := make([]*ProviderInfo, len(g.fallback))
	copy(fallback, g.fallback)
	g.chainMu.RUnlock()

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
	g.spentToday.Add(cost)
	if req.TaskType != "" {
		g.spentByTaskToday[req.TaskType] += cost
	}
	if req.Agent != "" {
		g.spentByAgentToday[req.Agent] += cost // per-identity ledger (M793)
	}
	spent := g.spentToday.Load()
	ceiling := g.effectiveCeilingLocked()
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
			"ceiling_mc":               ceiling,
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
	g.rolloverIfNeededLocked()
	ceiling := g.effectiveCeilingLocked()
	spent := g.spentToday.Load()
	g.mu.Unlock()
	if ceiling <= 0 {
		return false, spent, 0
	}
	return spent >= ceiling, spent, ceiling
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
func (g *Governor) Snapshot() BudgetSnapshot {
	g.mu.Lock()
	g.rolloverIfNeededLocked()
	snap := BudgetSnapshot{
		UTCDate:           g.today,
		SpentMicrocents:   g.spentToday.Load(),
		CeilingMicrocents: g.effectiveCeilingLocked(),
		StrictPricing:     g.cfg.StrictPricing,
	}
	g.mu.Unlock()
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
