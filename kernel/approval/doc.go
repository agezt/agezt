// SPDX-License-Identifier: MIT

// Package approval is the human-in-the-loop pause point. When Edict's
// trust ladder lands on an Ask-class level (L1..L3) and the engine is
// configured to actually prompt, the agent tool-loop suspends, the
// Registry emits an `approval.requested` event, and the caller blocks
// on Submit until either:
//
//   - an out-of-band caller (agt approve/deny over the control plane,
//     or — later — Telegram, web UI, Pulse) calls Resolve, or
//   - the per-request timeout fires (auto-deny with Reason=timeout).
//
// All four outcomes (granted / denied / timeout / cancelled) are
// journaled with the original CorrelationID so `agt why` walks the
// chain from the originating task to the approval verdict.
//
// SPEC-06 §3.4 defines this surface. M1.d ships the kernel + control-
// plane path; channel-routed prompts (Telegram, in-IDE) land later by
// implementing the same Resolve API.
package approval
