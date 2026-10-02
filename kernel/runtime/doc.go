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
// The package is large by design — it is the composition root — and is
// internally split into sub-packages that own distinct concerns:
// runexec (the Runner and the 42-entry KernelAPI interface), lifecycle
// (Halt/Resume/Cancel/Drain), accessors (read-only getters), compose
// (store opening), and types (shared value types). The god-file split into
// those sub-packages is recorded in .project/SPRINT-2026-09-REFACTOR-REPORT.md
// and in the Day-12…Day-211 split commits.
package runtime
