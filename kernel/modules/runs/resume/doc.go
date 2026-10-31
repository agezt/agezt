// SPDX-License-Identifier: MIT

// Package resume persists a durable "ticket" per in-flight root run so the
// daemon can pick the work back up after a restart — whether the daemon was
// stopped/started, self-updated, or hard-killed. Today a shutdown cancels every
// run (Halt → task.failed(reason=canceled)) and the work is lost; a ticket lets
// the run be re-dispatched with its accumulated conversation so it continues
// from where it left off instead of being abandoned (M1002).
//
// A ticket is written when a root run starts, its conversation snapshot is
// refreshed at each safe iteration boundary, and it is deleted on clean
// termination. If the daemon dies with the ticket still present, the boot-time
// resumer re-dispatches it.
//
// The package is deliberately dependency-light: it imports only kernel/contract
// llm (for the serialized Message slice) and internal/atomicfile. Everything
// else a run needs at resume time — trust ceiling, cost cap, wake context — is
// flattened to primitives so this package never imports kernel/runtime (which
// imports it), avoiding an import cycle. The runtime assembles a Ticket from
// its context values; the resumer (in the daemon, where the context builders
// live) rebuilds a run context from the flattened fields.
//
// # Why this package is the runs module's
//
// The target architecture gives the RUNS module the "run registry, resume
// tickets, steering/interventions" (architecture/20-target-architecture.md §4).
// The durable ticket is that module's DATA — it has a store, a lifecycle
// (active → suspended → deleted) and a quarantine, and nothing else in the
// daemon owns any of it. A rule that owns a store is a MODULE's rule
// (the counterpart of W3.73's "a rule that owns no store is the platform's"),
// and this one owns the run's own durable state.
//
// The store's FILE lives in the daemon's base directory, and the kernel's
// composition root opens it (kernel/runtime/compose.go) the same way it opens
// the journal, the state store and the config center. This package is the
// module's part: the rows, the store, and the rules that judge them.
package resume
