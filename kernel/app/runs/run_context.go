// SPDX-License-Identifier: MIT

package runs

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/runtime"
)

// A run's CONTEXT carries what the run is: the agent's profile (which binds
// the tool allow/deny, the trust ceiling, the standing instructions, the
// memory scope and the workdir), the cost ceiling the run may spend, the
// provenance of its wake, and the model chain and tool allowlist it runs
// under.
//
// The APPLICATION owns that, and only the application: the host that starts a
// run (the control plane's dispatcher, the roster's operator wake) decides
// WHY a run exists and WHICH agent runs it, but it must not have to name the
// runtime's module to say so — an adapter that reaches into a module for the
// run's own decoration is an adapter that has learned the module's
// mechanics.
//
// So the host calls the functions below, and the module's internals stay
// behind them.

// Decorate applies the run's profile, the provenance of its wake and its cost
// ceiling to ctx.
//
// The order matters for the reader, not for the values: the profile binds
// the run, the wake says why it exists, and the ceiling is layered on top so
// an explicit budget wins over the profile's own.
func Decorate(ctx context.Context, p agentapi.Profile, wake agentapi.WakeContext, maxCostMc int64) context.Context {
	ctx = runtime.WithAgentProfile(ctx, p)
	ctx = agentapi.WithWakeContext(ctx, wake)
	if maxCostMc > 0 {
		ctx = runtime.WithMaxCost(ctx, maxCostMc)
	}
	return ctx
}

// DecorateModels applies the run's ordered model fallback chain: the first
// entry is the primary the run asks for, the rest are what the Governor
// falls back to.
//
// An empty chain is a no-op — there is no primary to name, and a run with no
// primary keeps the daemon's default.
func DecorateModels(ctx context.Context, chain []string) context.Context {
	if len(chain) == 0 {
		return ctx
	}
	ctx = runtime.WithModel(ctx, chain[0])
	return runtime.WithModelChain(ctx, chain)
}

// DecorateTools restricts the run to the named tools. A non-nil slice —
// including an EMPTY one — activates the restriction; an unrestricted run is
// simply one whose tools are never decorated.
//
// The allowlist travels in the contract layer (toolapi), so the application
// states it without naming the runtime's module.
func DecorateTools(ctx context.Context, tools []string) context.Context {
	return toolapi.WithTools(ctx, tools)
}
