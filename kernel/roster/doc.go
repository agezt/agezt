// SPDX-License-Identifier: MIT

// Package roster is the durable agent roster (M783): named, persistent agent
// profiles — an identity ("researcher", "ops-watcher") with its own soul
// (system prompt), model (+ ordered fallbacks), default task type, per-run
// spend ceiling, memory scope, and workspace subdirectory. A profile is the
// durable HOME for everything that until now lived per-run: `agt run --agent
// researcher` runs AS that agent, and future arcs attach per-agent messaging,
// budgets, and tool grants to the same identity.
//
// Storage mirrors kernel/standing: a single JSON file rewritten atomically on
// change, safe for concurrent use; every lifecycle mutation is journaled by
// the kernel (roster.created/updated/removed) so `agt why` can explain how an
// agent came to exist.
package roster
