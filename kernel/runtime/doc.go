// SPDX-License-Identifier: MIT

// Package runtime wires the kernel subsystems (journal + state + bus +
// agent loop + providers + tools) into a single Kernel that the daemon
// hosts and the control plane drives.
//
// Boundary note: runtime is the composition root and thin adapter layer for
// the running Agezt process. It may temporarily host orchestration helpers
// while boundaries are being extracted, but long-term feature-specific logic
// should live in narrower domain packages (delegation, workflow execution,
// tool execution, context selection, etc.) with runtime assembling and owning
// the services.
//
// One Kernel per Agezt process. Concurrent Run calls are allowed (each
// gets its own correlation_id and ctx); Halt cancels every in-flight run
// and prevents new ones until Resume.
//
// The JOURNAL's provenance walks are the journal's own (W3.76ad): `Why`,
// `Causes`, `ParentOf` and `Verify` answer over the event log, and the event
// log is the host's own file (kernel/journal). The daemon holds no verb per
// journal operation any more: a caller that needs a walk asks the journal,
// through the daemon's own journal door (`k.Journal().Why(…)`).
//
// The RUN'S TOOL FOLD is the journal's too (W3.76aj): counting one run's tool
// results — and collecting the tool names, in order — is a walk over the event
// log, so the log answers it (`k.Journal().FoldTools(corr)`). The daemon used
// to hold its own copy of it (FoldRunTools) and the run engine asked the host
// for the fold: a second spelling of one family, and a hop the host never had
// to keep.
//
// The package is large by design — it is the composition root — and is
// internally split into sub-packages that own distinct concerns:
// runner (the Runner and the 23-entry KernelAPI interface), lifecycle
// (Halt/Resume/Cancel/Drain), accessors (read-only getters), compose
// (store opening), and types (shared value types). The god-file split into
// those sub-packages is recorded in .project/SPRINT-2026-09-REFACTOR-REPORT.md
// and in the Day-12…Day-211 split commits.
package runtime
