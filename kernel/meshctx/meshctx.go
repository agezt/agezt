// SPDX-License-Identifier: MIT

// Package meshctx carries the cross-node delegation hop count for the M8 mesh.
//
// A `remote_run` task is handed from one Agezt node to a peer over the peer's REST
// /api/v1/runs surface. Without a bound, a misconfigured or adversarial mesh can loop
// forever — node A delegates to B, B delegates back to A, and so on — each hop a real
// governed run that costs money and never terminates.
//
// The guard is a hop counter that travels with the delegation:
//   - The delegating node's `remote_run` reads the current run's hop from the context
//     and sends it +1 in the HopHeader on its POST to the peer.
//   - The receiving node's REST handler reads HopHeader; if it exceeds MaxHops the run
//     is refused, otherwise the hop is stored in the run context so that node's own
//     `remote_run` (if it fires) forwards hop+1 in turn.
//
// A run that did NOT arrive from a peer (a local `agt run`, a schedule, a channel) has
// no header, so Hop returns 0 and the chain starts fresh.
package meshctx

import "context"

// MaxHops bounds the length of a cross-node delegation chain. A run arriving with a
// hop above this is refused. Real delegation chains are short; 8 is generous headroom
// while still terminating a runaway loop quickly.
const MaxHops = 8

// HopHeader is the HTTP header carrying the delegation hop count between nodes.
const HopHeader = "X-Agezt-Mesh-Hop"

type ctxKey struct{}

// WithHop returns a context carrying the delegation hop count for the current run.
// A negative n is clamped to 0.
func WithHop(ctx context.Context, n int) context.Context {
	if n < 0 {
		n = 0
	}
	return context.WithValue(ctx, ctxKey{}, n)
}

// Hop returns the delegation hop count carried by ctx, or 0 when none is set (a run
// that did not arrive via a peer delegation).
func Hop(ctx context.Context) int {
	if n, ok := ctx.Value(ctxKey{}).(int); ok {
		return n
	}
	return 0
}
