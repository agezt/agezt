// SPDX-License-Identifier: MIT

// Package systemtasks holds the executors behind cadence's built-in system
// tasks (catalog_sync, artifact_collect, memory_clean, memory_tidy, log_clean,
// graveyard_scan, profile_distill) — the daemon-side maintenance work a
// schedule entry with Target=system_task dispatches (Phase 2.6 extraction from
// cmd/agezt; the catalogue + validation already lived in kernel/cadence).
//
// IMPORT-CYCLE CONSTRAINT: systemtasks imports kernel/runtime (the executors
// run against a live kernel). Nothing in kernel/cadence or kernel/runtime may
// EVER import systemtasks — the dependency arrow points one way only
// (cadence -> catalogue metadata; systemtasks -> cadence + runtime; the daemon
// wires the two together at its buildCadence dispatch site).
package systemtasks
