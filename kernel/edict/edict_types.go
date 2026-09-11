// SPDX-License-Identifier: MIT

package edict

// TrustLevel + AskPolicy String/Parse helpers. Carved out of edict.go
// during the Day 35 god file split #1 so the main file can focus on
// Engine + match helpers.

import (
	"fmt"
	"strings"
)

func (l TrustLevel) String() string {
	switch l {
	case LevelDeny:
		return "L0"
	case LevelAsk:
		return "L1"
	case LevelAskFirst:
		return "L2"
	case LevelAskScoped:
		return "L3"
	case LevelAllow:
		return "L4"
	default:
		return fmt.Sprintf("L?(%d)", int(l))
	}
}

// ParseTrustLevel parses an operator-facing trust-level string into a
// TrustLevel. It accepts the canonical "L0".."L4" labels (case-insensitive,
// the same vocabulary TrustLevel.String() emits) and the word aliases
// deny/ask/askfirst/askscoped/allow, so `agt edict level shell allow` and
// `... shell L4` are equivalent. Unknown input is an error, not a default —
// a typo must never silently land a capability at the wrong level.
func ParseTrustLevel(s string) (TrustLevel, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "l0", "deny":
		return LevelDeny, nil
	case "l1", "ask":
		return LevelAsk, nil
	case "l2", "askfirst", "ask-first":
		return LevelAskFirst, nil
	case "l3", "askscoped", "ask-scoped":
		return LevelAskScoped, nil
	case "l4", "allow":
		return LevelAllow, nil
	}
	return 0, fmt.Errorf("edict: unknown trust level %q (want L0..L4 or deny/ask/askfirst/askscoped/allow)", s)
}

// Decision is the engine's final operational decision.
type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
)

// Outcome is the full result of Decide. The runtime journals Outcomes
// as policy.decision events.
type Outcome struct {
	Decision   Decision
	Capability Capability
	Level      TrustLevel
	Reason     string
	// HardDenied is true iff a hard-deny rule matched, regardless of
	// trust level. A hard-denied Outcome always has Decision=Deny.
	HardDenied bool
	// HardDenyRule, when HardDenied, names the matching rule.
	HardDenyRule string
	// WouldAsk is true when the trust level was Ask-class (L1..L3) and
	// AskPolicy folded it. Used by the runtime to flag the journal entry
	// even though the operational decision was Allow.
	WouldAsk bool
	// RequiresApproval is true when AskPolicy=AskPrompt landed on an
	// Ask-class level. The runtime should pause the tool-loop and
	// submit an approval.Request; only after a grant should the call
	// proceed. Decision is left as DecisionDeny in this case so that
	// callers who ignore RequiresApproval still default-fail-closed.
	RequiresApproval bool
}

// AskPolicy controls how the engine resolves Ask-class levels (L1..L3): fold
// them into Allow (unattended runs), fold them into Deny (strict), or flag them
// for live human approval. The last is fully wired — AskPrompt sets
// RequiresApproval and the runtime routes the call through approval.Registry,
// blocking until an operator decides (see kernel/runtime policyHook).
type AskPolicy int

const (
	// AskAllow folds Ask into Allow with WouldAsk=true. Default; lets the
	// MVP build progress without an approver. The journal records every
	// would-have-asked moment so audits remain honest.
	AskAllow AskPolicy = iota
	// AskDeny folds Ask into Deny. Strict mode; only L4 calls pass.
	AskDeny
	// AskPrompt marks Ask-class verdicts with RequiresApproval=true so
	// the runtime can route a real prompt via approval.Registry. Until
	// the operator decides, the tool call must not proceed. Decision is
	// returned as Deny so any caller that ignores RequiresApproval will
	// fail closed.
	AskPrompt
)

// String returns the operator-facing label for an AskPolicy — the same
// vocabulary AGEZT_APPROVAL_MODE accepts and `agt edict show` prints.
func (p AskPolicy) String() string {
	switch p {
	case AskAllow:
		return "allow"
	case AskDeny:
		return "deny"
	case AskPrompt:
		return "prompt"
	default:
		return "unknown"
	}
}

// ParseAskPolicy parses an operator-facing approval-mode string into an
// AskPolicy. Accepts allow/deny/prompt (case-insensitive); unknown input
// is an error, never a silent default — a typo must not quietly flip the
// daemon into a different approval posture.
func ParseAskPolicy(s string) (AskPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "allow":
		return AskAllow, nil
	case "deny":
		return AskDeny, nil
	case "prompt":
		return AskPrompt, nil
	}
	return 0, fmt.Errorf("edict: unknown approval mode %q (want allow/deny/prompt)", s)
}

// HardDenyRule is a single hard-deny pattern.
type HardDenyRule struct {
	Name      string       // human label, included in the Outcome
	Substring string       // case-insensitive substring match against input
	AppliesTo []Capability // empty = applies to every capability
}

// matches reports whether r fires against the input under the named cap.
