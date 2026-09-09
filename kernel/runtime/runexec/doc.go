// SPDX-License-Identifier: MIT

// Package runexec owns the kernel's run engine. The Runner is
// the public surface for the linear entry points (Run / RunAssured /
// RunWith / RunWithRetry) and the journal re-exports (Why / Causes /
// ParentOf / Verify), reached through the KernelAPI interface so
// runexec stays import-clean of kernel/runtime.
//
// Extracted from kernel/runtime as the next step of the Day 12-23
// sub-package split: lifecycle (Day 12) → accessors (Day 13-19) →
// types (Day 17) → compose (Day 20) → runexec (Day 21-23).
//
// Day 23 slice: Runner ships Run / RunAssured / RunWith (delegate
// to *Kernel) + RunWithRetry (body migrated) + the four journal
// re-exports. The lock-ordering invariant documented on Kernel
// is the load-bearing refactor — any new locking site added in
// this file MUST respect:
//
//	configMu (light config) < runsMu < fanoutMu < treeMu < steersMu < spawnsMu < mcpMu
//
// The deferred cleanup at the bottom of *Kernel.RunWith takes
// all five (runsMu → fanoutMu → treeMu → steersMu → spawnsMu) in
// this exact order and releases them in reverse. The 260-line
// RunWith body stays on *Kernel for now (see Runner.RunWith
// comment) because it touches ~30 private fields/methods that
// would bloat the KernelAPI interface from 17 to ~80 entries
// OR require breaking the existing dependency arrow by relocating
// the Runner construction out of kernel/runtime/compose.go.
//
// Circular-import guard: like lifecycle and accessors, runexec
// does not import kernel/runtime. The Runner speaks to the host
// through the KernelAPI interface (api.go). Every method the
// Runner calls has a public counterpart on *Kernel — added as
// wrappers in Day 22-23 (ClaimResumeTicket, FinalizeResumeTicket,
// AgentSlugFromCtx, AgentRetryPolicyFromCtx, RetryReason,
// AgentRetryable, RetryDelay, RunWithRetry).
package runexec
