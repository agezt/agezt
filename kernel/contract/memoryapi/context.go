// SPDX-License-Identifier: MIT

package memoryapi

// Provenance: the memory context seam (W3.57). A run's correlation id and a
//             run's per-agent memory scope are threaded through the context
//             by the layers above the module and read by the module's own
//             tool — so the seam travels with the rows it scopes, or the
//             layers above would have to name the module to set it.
//
// The keys live HERE and only here: a context key is an identity, and two
// packages that each define one would not see each other's values.

import "context"

type ctxKey int

const (
	ctxKeyCorrelation ctxKey = iota
	ctxKeyScope
)

// WithCorrelation returns a child context carrying the run's correlation id.
func WithCorrelation(ctx context.Context, corr string) context.Context {
	return context.WithValue(ctx, ctxKeyCorrelation, corr)
}

// CorrelationFrom extracts the correlation id set by WithCorrelation.
func CorrelationFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyCorrelation).(string); ok {
		return v
	}
	return ""
}

// WithScope returns a child context carrying the run's per-agent memory scope
// (M786): when a run executes AS a named agent (M783), its recalls — the
// context injection and the memory tool — default to this scope, so the agent
// sees its own private notes on top of shared memory without having to name
// itself. Writes default to this scope too (M915 — each agent keeps its own
// memory; the shared brain is opt-in via the tool's shared=true and kept
// selective). The explicit tool scope param always wins over this default.
func WithScope(ctx context.Context, scope string) context.Context {
	if scope == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyScope, scope)
}

// ScopeFrom extracts the per-agent memory scope set by WithScope ("" = none).
func ScopeFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyScope).(string); ok {
		return v
	}
	return ""
}
