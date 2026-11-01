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
// spawn handles. The daemon keeps the mechanism: it builds the child's loop
// configuration and runs the child loop, because the loop itself is the next
// family to move (kernel/agent, W3.76g) and the loop configuration the one
// after that (kernel/runtime/loopconfig.go).
//
// The port is the Host interface (host.go): twelve doors the daemon opens to
// the module. Nine of them are reads or writes of live daemon state (the
// roster, the live model, the live identity, the halt latch, the correlation
// mint, the bus, the journal, the steering registry, the skill forge). One
// returns the child's base loop configuration and one returns the daemon's
// halt error, so the module fails with the SAME error identity the daemon
// used. The rest of the collaboration is in Deps: the live config provider,
// the child-runner callback, the live-run registry and the sleeper.
//
// The module never imports the daemon: the daemon imports the module.
package delegation
