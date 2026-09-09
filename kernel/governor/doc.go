// SPDX-License-Identifier: MIT

// Package governor is the LLM routing brain. It implements
// agent.Provider, so every call from the agent loop transparently
// flows through selection, fallback, budget tracking, and per-task
// caps without the rest of the kernel knowing it exists. Three
// pieces: (1) the Registry of providers and their models, hot-
// reloadable via Replace(); (2) the chain construction (subscription-
// first → quality → cost → latency, with a local floor always
// eligible per DECISIONS C2); (3) the budget/pricing engine, which
// is integer-microcent (DECISIONS C1) so float drift is impossible.
// A circuit breaker skips flaky providers; a per-provider cache
// serves exact repeats at zero token cost; a strict-pricing gate
// refuses a model the catalog + fallback table cannot price.
package governor
