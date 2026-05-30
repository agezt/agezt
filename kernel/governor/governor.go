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
}

// Governor is the per-task routing + budget layer.
type Governor struct {
	cfg Config

	mu               sync.Mutex
	spentToday       int64            // microcents (global)
	spentByTaskToday map[string]int64 // microcents per task type (M1.zz)
	today            string           // YYYY-MM-DD UTC

	// Stable ordering for routing: primary chain + fallback chain.
	primary  []*ProviderInfo
	fallback []*ProviderInfo
}

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
	g.mu.Lock()
	defer g.mu.Unlock()
	g.primary = g.primary[:0]
	g.fallback = g.fallback[:0]
	for _, p := range g.cfg.Registry.All() {
		if p.IsFallback {
			g.fallback = append(g.fallback, p)
		} else {
			g.primary = append(g.primary, p)
		}
	}
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

	// Budget pre-check (don't even attempt if already past the ceiling).
	if exceeded, spent, ceiling := g.budgetExceeded(); exceeded {
		g.publish(event.Spec{
			Subject: "governor.budget",
			Kind:    event.KindBudgetExceeded,
			Actor:   "governor",
			Payload: map[string]any{"spent_microcents": spent, "ceiling_microcents": ceiling},
		})
		return nil, fmt.Errorf("%w (spent=%d, ceiling=%d microcents)", ErrBudgetExceeded, spent, ceiling)
	}

	// Per-task-type budget pre-check (M1.zz). Only fires when the
	// request carries a TaskType AND that type has a configured cap.
	if req.TaskType != "" {
		if exceeded, spent, cap := g.taskBudgetExceeded(req.TaskType); exceeded {
			g.publish(event.Spec{
				Subject: "governor.budget",
				Kind:    event.KindBudgetExceeded,
				Actor:   "governor",
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

	chain := g.routeChain(req)
	if len(chain) == 0 {
		return nil, errors.New("governor: no eligible providers")
	}

	// Initial routing decision (first pick). task_type is included
	// so operators using `agt pulse --kind routing.decision` can see
	// which task-type overrides actually fired.
	g.publish(event.Spec{
		Subject: "governor.route",
		Kind:    event.KindRoutingDecision,
		Actor:   "governor",
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
				Subject: "governor.fallback",
				Kind:    event.KindProviderFallback,
				Actor:   "governor",
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
// wiring close the loop without circular-init gymnastics.
//
// MUST be called before the first Complete; the bus pointer is read
// without a lock on the hot path.
func (g *Governor) SetBus(b *bus.Bus) {
	g.cfg.Bus = b
}

// DailyCeilingMicrocents returns the configured global daily cap (0 =
// unlimited). Used by the daemon to derive per-tenant ceilings.
func (g *Governor) DailyCeilingMicrocents() int64 {
	return g.cfg.DailyCeilingMicrocents
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
	ncfg := g.cfg // copy: shares Registry/Bus pointers, copies scalars/maps refs
	ncfg.DailyCeilingMicrocents = ceiling
	return New(ncfg)
}

// Providers returns a snapshot of the routing chain (primary first,
// fallback last). Used by the daemon banner.
func (g *Governor) Providers() []*ProviderInfo {
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
	primary := make([]*ProviderInfo, len(g.primary))
	copy(primary, g.primary)
	slices.SortStableFunc(primary, func(a, b *ProviderInfo) int {
		return authModePriority(a.AuthMode) - authModePriority(b.AuthMode)
	})
	chain := make([]*ProviderInfo, 0, len(primary)+len(g.fallback))
	chain = append(chain, primary...)
	chain = append(chain, g.fallback...)
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
	cost := costMicrocents(model, resp.Usage.InputTokens, resp.Usage.OutputTokens)

	g.mu.Lock()
	g.rolloverIfNeededLocked()
	g.spentToday += cost
	if req.TaskType != "" {
		g.spentByTaskToday[req.TaskType] += cost
	}
	spent := g.spentToday
	g.mu.Unlock()

	g.publish(event.Spec{
		Subject: "governor.budget",
		Kind:    event.KindBudgetConsumed,
		Actor:   "governor",
		Payload: map[string]any{
			"provider":        p.Name,
			"model":           model,
			"input_tokens":    resp.Usage.InputTokens,
			"output_tokens":   resp.Usage.OutputTokens,
			"cost_microcents": cost,
			"spent_today_mc":  spent,
			"ceiling_mc":      g.cfg.DailyCeilingMicrocents,
		},
	})
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
	if g.cfg.Bus == nil {
		return
	}
	_, _ = g.cfg.Bus.Publish(spec)
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
