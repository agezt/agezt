// SPDX-License-Identifier: MIT

// Package selfrepair wires the deterministic doctor/auto-repair coordinator
// (Phase 2.6 extraction from cmd/agezt): it subscribes to the reaper pulse
// observer, claims broken/degraded/routing-unstable agents, drives the
// overseer's repair source, and escalates through the mailbox + wake chain
// when a repair fails. The daemon arms it once at boot via WireAutoRepair.
//
// Import posture: selfrepair holds NO handle — it names the host's
// CAPABILITIES (the Ports struct) and the composition root hands them over.
// It speaks the agents module's own repair vocabulary (roster.RepairSource)
// and the runs module's api package for the run engine; it does not import the
// overseer PLUGIN — the daemon injects the repair source. Nothing in
// kernel/runtime may ever import selfrepair, and nothing in selfrepair may
// import kernel/runtime: that edge was the last one in the tree (W5.4bd).
package selfrepair
