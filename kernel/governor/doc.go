// SPDX-License-Identifier: MIT

// Package governor is the LLM routing brain. It implements agent.Provider,
// so every call from the agent loop transparently flows through selection,
// fallback, budget tracking, and per-task caps without the rest of the kernel
// knowing it exists.
//
// Three pieces: (1) the Registry of providers and their models,
// hot-reloadable via Replace(); (2) the chain construction (below); (3) the
// budget/pricing engine, which is integer-microcent (DECISIONS C1) so float
// drift is impossible. A circuit breaker skips flaky providers; a per-provider
// cache serves exact repeats at zero token cost; a strict-pricing gate refuses
// a model the catalog + fallback table cannot price.
//
// # Chain order
//
// routeChain orders the candidates by auth mode, cheapest-marginal first —
// this is the cost axis of DECISIONS C2 and it has shipped:
//
//	AuthSubscription  the caller has already paid (Anthropic Pro, ChatGPT
//	                  Plus, …), so a call costs $0 marginal
//	AuthLocal         Ollama and other local servers: no per-call cost, and no
//	                  rate limit shared with a paid key
//	AuthAPIKey        pay-per-token, tried only once the fixed-cost options
//	                  are ineligible or have failed
//	(fallback)        providers with IsFallback, always last, in registry
//	                  insertion order regardless of auth mode
//
// Unknown auth modes fold into AuthAPIKey's tier — cost-conservative, since
// anything unrecognised is assumed to bill per call.
//
// TaskRoutes and TaskRouteRequires (routes.go) reshape that order per task
// type: the former hoists listed providers to the front and still falls
// through on failure, the latter hard-restricts the chain. A per-request
// RouteOptions.PreferredProvider, when set and registered, is tried first.
//
// Quality and latency are NOT yet ordering dimensions — only cost is. They
// arrive with the model-catalog sync, which is why the chain needs the
// catalog in the first place. An earlier version of this comment described
// the order as "subscription-first -> quality -> cost -> latency", which
// described the intended end state rather than the shipped one.
package governor
