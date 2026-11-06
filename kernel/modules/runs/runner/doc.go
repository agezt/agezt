// SPDX-License-Identifier: MIT

// Package runner owns the kernel's run engine. The Runner is
// the public surface for the linear entry points (Run / RunAssured /
// RunWith / RunWithRetry) and the journal re-exports (Why / Causes /
// ParentOf / Verify), reached through the KernelAPI interface so the
// runner stays import-clean of the daemon.
//
// # WHY IT LIVES HERE
//
// The run engine is the runs module's (W3.76j). It used to live at
// kernel/runtime/runexec — a sub-package of the daemon, reachable only
// through the daemon's own files. The move makes the tree the statement: the
// thing that executes a run belongs to the module that owns runs.
//
// The seam did not change, because it was already the right one: the Runner
// speaks to its host through the KernelAPI interface (api.go), and the
// daemon implements that interface. The daemon keeps the doors — the
// *Kernel.Run / RunAssured / RunWith / RunWithRetry entry points and the
// run-state setup — and the module keeps the engine.
//
// W3.76ac moved the run doors themselves into the module's own face
// (api.RunnerService), and W3.76ae moved the run's live STATE — the
// steering registry and the per-run setup and cleanup — into the module's
// own state package (kernel/modules/runs/state). What is left of the seam
// is what the HOST owns: the correlation id a run carries, the resume
// handshake, the prompt assembly and the retry policy reads.
//
// The lock-ordering invariant documented on Kernel is the load-bearing
// constraint: any new locking site added in this package MUST respect
//
//	registry lock < configMu < the live-run state's own lock
//
// The live-run state's own lock is the module's own (W3.76ae): the steering
// registry and the per-run registration live in kernel/modules/runs/state,
// and that package takes its lock INSIDE the registry's lock. The deferred
// cleanup in RunWith releases the run's live state through the module's
// state package, in that order.
//
// Circular-import guard: like the loop, the runner does not import the
// daemon. Every method the Runner calls has a counterpart on the host's
// KernelAPI implementation — added as wrappers when the Runner was
// extracted (ClaimResumeTicket, FinalizeResumeTicket, AgentSlugFromCtx,
// AgentRetryPolicyFromCtx, RetryReason, AgentRetryable, RetryDelay,
// RunWithRetry).
//
// History: extracted from kernel/runtime as part of the Day 12-23
// sub-package split: lifecycle (Day 12) → accessors (Day 13-19) → types
// (Day 17) → compose (Day 20) → runexec (Day 21-23).
package runner
