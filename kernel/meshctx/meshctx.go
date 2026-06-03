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

import (
	"context"
	"os"
	"strconv"
	"strings"
)

// MaxHops is the DEFAULT bound on the length of a cross-node delegation chain. A run
// arriving with a hop above the effective limit is refused. Real delegation chains are
// short; 8 is generous headroom while still terminating a runaway loop quickly. The
// effective limit is operator-tunable per node via EnvMaxHops (see MaxHopsFromEnv).
const MaxHops = 8

// maxConfigurableHops caps the operator override so a typo like "100000" can't defeat
// the guard's purpose. Above this (or below 1, or unparseable) the default is used.
const maxConfigurableHops = 64

// EnvMaxHops is the env var that overrides MaxHops for this node. Hardcoded (rather
// than brand.EnvPrefix+…) to keep this leaf package dependency-free; it matches the
// AGEZT_ prefix and is registered in controlplane.configEnvVars for `agt config show`.
const EnvMaxHops = "AGEZT_MESH_MAX_HOPS"

// HopHeader is the HTTP header carrying the delegation hop count between nodes.
const HopHeader = "X-Agezt-Mesh-Hop"

// MaxConfigurableHops is the largest hop limit an operator may configure via EnvMaxHops.
const MaxConfigurableHops = maxConfigurableHops

// MaxHopsFromEnv returns the effective hop limit for this node: the EnvMaxHops override
// when it is a valid integer in [1, MaxConfigurableHops], otherwise the MaxHops default.
// Each node enforces its own limit; the receiving node is authoritative.
func MaxHopsFromEnv() int {
	eff, _, _ := MaxHopsConfig()
	return eff
}

// MaxHopsConfig reports the effective hop limit, the raw EnvMaxHops value, and whether a
// SET value is a valid override. When EnvMaxHops is unset, raw is "" and validOverride is
// true (the default is in effect). When it is set but not an integer in
// [1, MaxConfigurableHops], the default is returned with validOverride=false — so a caller
// (e.g. `agt doctor`) can flag a typo that would otherwise silently fall back.
func MaxHopsConfig() (effective int, raw string, validOverride bool) {
	raw = strings.TrimSpace(os.Getenv(EnvMaxHops))
	if raw == "" {
		return MaxHops, "", true
	}
	if n, err := strconv.Atoi(raw); err == nil && n >= 1 && n <= maxConfigurableHops {
		return n, raw, true
	}
	return MaxHops, raw, false
}

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
