// SPDX-License-Identifier: MIT

// Package schedule is the in-process cronjob tool. It lets an agent create
// typed future work — wake this agent task, run a stored workflow, run daemon
// maintenance, or invoke a registered tool — by writing to the daemon's
// persistent cadence store, the same store the `agt schedule` CLI and the
// AGEZT_SCHEDULE env jobs use. A scheduled job fires through its target
// executor at its due time (M634).
//
// This is the autonomy primitive that turns a reactive assistant into a
// proactive one: an agent can install a visible future job without embedding
// identity instructions inside the schedule. Schedules it creates are tagged
// source="agent" so an operator can see and prune them (`agt schedule list`).
//
// The tool is created unbound and Bound to the live store after the kernel
// opens (the store is the kernel's), mirroring the notify tool's lifecycle.
package schedule
