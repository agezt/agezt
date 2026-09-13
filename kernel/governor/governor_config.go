// SPDX-License-Identifier: MIT

package governor

// Governor configuration: Config struct with all routing/budget/cache knobs.
// Carved out of governor.go during the Day 149 god-file split so the main
// file can focus on Governor struct + lifecycle + errors.
// Public API unchanged.

import (
	"time"

	"github.com/agezt/agezt/kernel/bus"
)

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
	// (M193). Off by default. When off, a model with no known price (missing
	// from the catalog AND the fallback table) is charged the conservative
	// unpricedFallbackPrice and journals budget.unpriced, so it consumes
	// ledger headroom like any other model — it used to be charged $0, which
	// silently bypassed the daily, per-task and per-agent budgets at once
	// (BIZ-001). When on, such a request is refused with ErrUnpricedModel
	// BEFORE any provider call, so an operator who would rather stop than
	// bill an estimate can guarantee every billed call has a real price.
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

	// BreakerThreshold is how many consecutive fall-back failures trip a
	// provider's circuit breaker open, after which the Governor skips it for
	// BreakerCooldown and serves from the rest of the chain (M997). 0 →
	// DefaultBreakerThreshold; negative → breaker disabled.
	BreakerThreshold int
	// BreakerCooldown is how long a tripped provider stays open before one
	// half-open probe is allowed. 0 → DefaultBreakerCooldown.
	BreakerCooldown time.Duration
}

// DefaultProviderRetries is the default number of in-place retries per
// provider on a transient error (M882): 2 retries → 3 attempts total,
// ~0.5s + ~1s of backoff — enough to ride out a rate-limit window without
// meaningfully delaying a genuine outage's fallback.
const DefaultProviderRetries = 2

// DefaultRetryBaseDelay is the first in-place retry's backoff (M882).
const DefaultRetryBaseDelay = 500 * time.Millisecond

// Governor is the per-task routing + budget layer.
