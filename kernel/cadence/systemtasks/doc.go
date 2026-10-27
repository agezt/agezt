// SPDX-License-Identifier: MIT

// Package systemtasks holds the executors behind cadence's built-in system
// tasks (catalog_sync, artifact_collect, memory_clean, memory_tidy, log_clean,
// graveyard_scan, profile_distill) — the daemon-side maintenance work a
// schedule entry with Target=system_task dispatches (Phase 2.6 extraction from
// cmd/agezt; the catalogue + validation already lived in kernel/cadence).
//
// IMPORT-CYCLE CONSTRAINT: nothing in kernel/cadence or kernel/runtime may
// EVER import systemtasks — the dependency arrow points one way only
// (cadence -> catalogue metadata; the daemon wires the two together at its
// buildCadence dispatch site).
//
// The executors run against a live daemon, and they used to run against a
// live KERNEL: this package imported kernel/runtime and every executor took
// the kernel as its argument (W5.4e). The executors still need the daemon's
// capabilities — the memory, the roster, the artifact index, the journal, the
// bus, the catalog — but they ask for them through the CONTRACT's ports
// (cadenceapi.SystemTaskPorts), which the kernel hands over. The arrow still
// points one way: the daemon asks the kernel for the ports and the module
// names the contract, never the kernel.
package systemtasks
