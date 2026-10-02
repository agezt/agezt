// SPDX-License-Identifier: MIT

// Package homeassistant is the in-process Home Assistant control tool. Where the
// homeassistant CHANNEL is outbound-only (it pushes a brief to a notify service),
// this TOOL makes the smart home READABLE and ACTIONABLE from inside an agent run:
// the agent can read entity states ("is the living-room light on?", "what's the
// thermostat set to?") and call services ("turn the porch light off", "set the
// bedroom to 20°C"). It turns Agezt into something that can actually act on the
// house, not just announce into it.
//
// Two operations against the HA REST API (developers.home-assistant.io/docs/api/rest):
//
//   - get_states  → GET {base}/api/states[/{entity_id}]   (read; low risk)
//   - call_service→ POST {base}/api/services/{domain}/{service}  (actuate; physical)
//
// Security (SPEC-04 §1.7) — this tool touches the physical world, so it is
// fail-closed on two independent axes:
//
//   - The HA URL and token are OPERATOR-pinned config; the agent never supplies
//     the host, so there is no SSRF / arbitrary-egress surface (unlike the http
//     tool, which is why this tool needs no netguard — the destination is fixed).
//   - call_service is gated by a SERVICE allowlist (e.g. "light.turn_on",
//     "climate.*"): empty → no service is callable. get_states is gated by a READ
//     entity allowlist (e.g. "sensor.*", "light.living_room", "*"): empty → no
//     state is readable, and a bulk read is FILTERED to the allowlist so a
//     prompt-injected agent can't enumerate the whole house. The two axes map to
//     distinct Edict capabilities (homeassistant.read = Allow by default,
//     homeassistant.call = AskFirst), so an operator can let the agent read freely
//     while still confirming every actuation.
//
// The token is never logged. Response bodies are size-capped before the model
// sees them. The HTTP client is injectable so behaviour is unit-testable without
// a live Home Assistant.
package homeassistant
