// SPDX-License-Identifier: MIT

package runctx

import "context"

// ResolveModel picks the model a run should ask for.
//
// The per-run override wins and is reported as EXPLICIT — the caller then
// carries it as the per-request model chain, so the governor's per-task chain
// cannot silently reroute an operator's deliberate pick (M931).
//
// The default is the HOST's: the daemon's live default model (hot-swappable,
// M816) retuned by the running agent's own overrides (M783). It is asked for
// LAZILY, because a run that carries its own model never needs the host's
// config at all — and the host's config is a copy under a lock.
//
// The rule is the RUN CONTEXT's (W3.76al): the per-run pick is the run
// context's own vocabulary (`WithModel`), and the run engine — which lives
// inside the module — used to ask the HOST to apply it, a round trip inside
// one module for a decision the context already holds.
func ResolveModel(ctx context.Context, defaultModel func() string) (string, bool) {
	if m := ModelFromCtx(ctx); m != "" {
		return m, true
	}
	return defaultModel(), false
}
