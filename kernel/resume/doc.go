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
// The package is deliberately dependency-light: it imports only kernel/agent
// (for the serialized Message slice). Everything else a run needs at resume
// time — trust ceiling, cost cap, wake context — is flattened to primitives so
// this package never imports kernel/runtime (which imports it), avoiding an
// import cycle. The runtime assembles a Ticket from its context values; the
// resumer (in the daemon, where the context builders live) rebuilds a run
// context from the flattened fields.
package resume
