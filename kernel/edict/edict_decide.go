// SPDX-License-Identifier: MIT

// Package edict: policy decision functions (Decide + DecideWithCeiling).
// Extracted from edict_engine.go during the Day-211 god-file split.
// Public API unchanged.
package edict


import (
	"fmt"
)
func (e *Engine) Decide(cap Capability, input string) Outcome {
	// No ceiling: LevelAllow (the max) clamps nothing, so behaviour is unchanged.
	return e.DecideWithCeiling(cap, input, LevelAllow)
}

// DecideWithCeiling is Decide with a per-call trust ceiling (SPEC-16 §4
// initiative.max_trust): the looked-up capability level is clamped to at most
// `ceiling` before the level→decision mapping, so a normally auto-allowed (L4)
// capability is downgraded to Ask (or, at ceiling L0, Deny) within a bounded
// context like a standing order. The hard-deny floor and unknown-capability
// default-deny are unaffected — a ceiling can only TIGHTEN, never loosen.
func (e *Engine) DecideWithCeiling(cap Capability, input string, ceiling TrustLevel) Outcome {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. Hard-deny always wins. Match against the decoded, normalized action — not
	// just the raw JSON tool-arg text. The model picks the command string, so it
	// could otherwise evade a floor rule by JSON-escaping a banned token
	// (`{"command":"rm -rf /"}`) or by padding whitespace (`rm  -rf /`); both
	// decode/normalize back to the banned form. denyCandidates returns the raw
	// input (no regression) plus each JSON string value with whitespace collapsed
	// (M173). A rule firing on ANY candidate denies.
	candidates := denyCandidates(input)
	for _, r := range e.hardDeny {
		for _, c := range candidates {
			if r.matches(cap, c) {
				return Outcome{
					Decision:     DecisionDeny,
					Capability:   cap,
					Level:        LevelDeny,
					Reason:       "hard-deny rule matched: " + r.Name,
					HardDenied:   true,
					HardDenyRule: r.Name,
				}
			}
		}
	}

	// 2. Look up the trust level for this capability.
	lvl, ok := e.levels[cap]
	if !ok {
		// Unknown capability. Default-deny is the strict, secure default (the
		// user must explicitly grant a level). But under UnknownAllow (M613, set
		// by AGEZT_ALLOW_ALL) an unconfigured capability is treated as L4 so
		// "allow everything" covers tools whose capability isn't in DefaultLevels
		// — including future plugin tools. Hard-deny already ran above, so the
		// catastrophe rails still hold.
		if !e.unknownAllow {
			return Outcome{
				Decision:   DecisionDeny,
				Capability: cap,
				Level:      LevelDeny,
				Reason:     fmt.Sprintf("no trust level configured for %q (default-deny)", cap),
			}
		}
		lvl = LevelAllow
	}

	// 2b. Clamp to the per-call ceiling (SPEC-16 §4): autonomy within this context
	// can be capped below the capability's configured level. Only ever tightens.
	ceilNote := ""
	if ceiling < lvl {
		lvl = ceiling
		ceilNote = fmt.Sprintf(" (clamped to ceiling %s)", ceiling)
	}

	// 3. Apply level → decision.
	switch lvl {
	case LevelDeny:
		return Outcome{
			Decision:   DecisionDeny,
			Capability: cap,
			Level:      lvl,
			Reason:     "capability set to L0 (deny)" + ceilNote,
		}
	case LevelAllow:
		return Outcome{
			Decision:   DecisionAllow,
			Capability: cap,
			Level:      lvl,
			Reason:     "capability set to L4 (allow)",
		}
	default: // L1..L3 — Ask
		switch e.askPolicy {
		case AskDeny:
			return Outcome{
				Decision:   DecisionDeny,
				Capability: cap,
				Level:      lvl,
				Reason:     fmt.Sprintf("level %s requires approval; AskPolicy=AskDeny", lvl) + ceilNote,
			}
		case AskPrompt:
			return Outcome{
				// Fail-closed default for callers that don't honour
				// RequiresApproval — the runtime overrides this after
				// a real grant.
				Decision:         DecisionDeny,
				Capability:       cap,
				Level:            lvl,
				Reason:           fmt.Sprintf("level %s; AskPolicy=AskPrompt → operator approval required", lvl) + ceilNote,
				WouldAsk:         true,
				RequiresApproval: true,
			}
		default: // AskAllow
			return Outcome{
				Decision:   DecisionAllow,
				Capability: cap,
				Level:      lvl,
				Reason:     fmt.Sprintf("level %s; AskPolicy=AskAllow (would prompt in MVP)", lvl) + ceilNote,
				WouldAsk:   true,
			}
		}
	}
}
