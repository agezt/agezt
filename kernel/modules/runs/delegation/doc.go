// SPDX-License-Identifier: MIT

// Package delegation is the RUNS MODULE's delegation engine: the policy that
// decides WHO may delegate WHAT, under WHICH limits, and with WHICH identity.
//
// It is the sixth slice of the run engine programme (W3.76). W3.76a took the
// durable tickets, W3.76b the steering, W3.76c the run context, W3.76d the run
// registry, W3.76e the prompt; this one takes the delegation engine — the
// family that used to sit in the daemon as kernel/runtime/subagent*.go (six
// files, 794 lines) plus the kernel/delegation vocabulary package (100
// lines).
//
// The split is POLICY versus MECHANISM. The module owns the policy: it
// resolves the named agent, picks the model chain, enforces the four bounds
// (depth, fan-out, tree total, spend), builds the child's context and
// identity, journals the spawn, drives the retry policy, and owns the async
// spawn handles. The daemon keeps the mechanism: it runs the child loop,
// because the loop itself is the next family to move (kernel/agent). The
// child's loop configuration is the runs module's rule too
// (kernel/modules/runs/loopcfg, W3.76g) — and since W3.77 the engine HOLDS
// that very service (Deps.LoopCfg) and asks it directly, the way the run
// engine holds it. The daemon used to hand the engine the child's base
// through the BaseLoopConfig door: a round trip inside one module.
//
// The port is the Host interface (host.go): eleven doors the daemon opens to
// the module. Nine of them are reads or writes of live daemon state (the
// roster, the live model, the live identity, the halt latch, the correlation
// mint, the bus, the journal, the steering registry, the skill forge). One
// returns the daemon's halt error, so the module fails with the SAME error
// identity the daemon used. The rest of the collaboration is in Deps: the
// live config provider, the child-runner callback, the prompt service, the
// loop configuration and the live-run registry.
//
// The module never imports the daemon: the daemon imports the module.
package delegation
