// SPDX-License-Identifier: MIT

// Package introspecttool is the in-process self-introspection tool: it lets the
// agent read the DAEMON's OWN live state in one call — a real health overview
// (uptime, halted, active runs, memory/world/skill counts, journal head,
// schedule/standing/approval posture, provider-fallback health, delegation
// ceilings), plus detailed listings of the schedules and standing orders that
// drive its autonomy (M682).
//
// This closes the introspection gap: the granular tools (memory, world, runs,
// skill) each read ONE slice, so a "summarise AGEZT's health every morning at 9"
// task had no single place to see the whole system and would resort to guessing
// (or web-searching "AGEZT"). `introspect` is that place — the agent can actually
// see everything that's running before it reports. Read-only; no mutation, no
// network.
package introspecttool
