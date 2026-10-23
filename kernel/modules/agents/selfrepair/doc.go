// SPDX-License-Identifier: MIT

// Package selfrepair wires the deterministic doctor/auto-repair coordinator
// (Phase 2.6 extraction from cmd/agezt): it subscribes to the reaper pulse
// observer, claims broken/degraded/routing-unstable agents, drives the
// overseer's repair source, and escalates through the mailbox + wake chain
// when a repair fails. The daemon arms it once at boot via WireAutoRepair.
//
// Import posture: selfrepair imports kernel/runtime and speaks the agents
// module's own repair vocabulary (roster.RepairSource); it does not import the
// overseer PLUGIN — the daemon injects the repair source. Nothing in
// kernel/runtime may ever import selfrepair.
package selfrepair
