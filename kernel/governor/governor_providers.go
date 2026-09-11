// SPDX-License-Identifier: MIT

package governor

// Governor provider routing helpers: Providers + sortPrimary +
// routeChain + applyModelRoute + authModePriority + the tail of
// routeChain (applyTaskRouteRequire call). Carved out of governor.go
// during the Day 29 god file split #2.

import (
	"slices"

	"github.com/agezt/agezt/kernel/agent"
)

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

