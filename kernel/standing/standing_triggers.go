// SPDX-License-Identifier: MIT

// Package standing: InitiativeMode helpers (validMode + MaxAutonomyTrust).
// Split from standing.go during Day 211 god-file refactor (#47).
// Public API unchanged.
package standing

import (
)

func validMode(m InitiativeMode) bool {
	switch m {
	case InitiativeInformOnly, InitiativeAsk, InitiativeActOrAsk:
		return true
	}
	return false
}

// MaxAutonomyTrust maps an initiative mode to the trust ceiling (Lx) that ENFORCES
// it on a firing (M999). The mode is the operator's autonomy dial; until now it was
// stored but never applied (only max_trust gated). The mapping:
//
//   - inform_only → "L0" (LevelDeny): no tools at all — the agent may reason and
//     brief from the trigger context, but cannot act. "Notice and tell me."
//   - ask         → "L1" (LevelAsk): every effectful call needs approval, so an
//     unattended firing can't silently change anything.
//   - act_or_ask / "" → ("", false): no extra cap — the order's own max_trust wins.
//     Empty mode preserves pre-M999 behavior (backward compatible).
//
// The fire path combines this with the order's max_trust by taking the MORE
// restrictive (lower) level. Pure, so it's testable without importing edict.
func (m InitiativeMode) MaxAutonomyTrust() (level string, capped bool) {
	switch m {
	case InitiativeInformOnly:
		return "L0", true
	case InitiativeAsk:
		return "L1", true
	default: // act_or_ask, "", or anything unrecognized → no extra cap
		return "", false
	}
}
