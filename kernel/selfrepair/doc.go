// SPDX-License-Identifier: MIT

// Package selfrepair wires the deterministic doctor/auto-repair coordinator
// (Phase 2.6 extraction from cmd/agezt): it subscribes to the reaper pulse
// observer, claims broken/degraded/routing-unstable agents, drives the
// overseertool repair source, and escalates through the mailbox + wake chain
// when a repair fails. The daemon arms it once at boot via WireAutoRepair.
//
// Import posture: selfrepair imports kernel/runtime and (like
// kernel/controlplane) the overseertool plugin as its repair source; nothing in
// kernel/runtime may ever import selfrepair.
package selfrepair
