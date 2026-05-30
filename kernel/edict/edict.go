// SPDX-License-Identifier: MIT

// Package edict is the policy engine + trust ladder
// (TASKS P1-EDICT-01..03; DECISIONS F3/F4).
//
// Two layers:
//
//  1. Hard-deny rules (DECISIONS F4) — pattern-matched against tool input;
//     ALWAYS deny regardless of trust level, never overridable. fork-bombs,
//     rm -rf /, mkfs, shutdown/reboot, audit-disable attempts.
//
//  2. Trust ladder (DECISIONS F3) — per-capability level L0..L4.
//     L0 deny · L1-L3 ask · L4 allow. M1 has no live approval routing, so
//     "ask" levels are folded by the engine's AskPolicy: AskAllow (default)
//     treats Ask as Allow + WouldAsk=true so the journal captures the
//     would-have-been-prompt; AskDeny treats Ask as Deny (strict mode).
//
// Every Decide call is intended to be journaled as a policy.decision event
// by the runtime; the engine itself does not journal so it stays a pure,
// easily-testable function.
package edict

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
)

// Capability identifies a class of action governed by policy. Capability
// strings are stable; downstream loggers/UIs depend on them.
type Capability string

const (
	CapShell        Capability = "shell"
	CapFileRead     Capability = "file.read"
	CapFileWrite    Capability = "file.write"
	CapFileDelete   Capability = "file.delete"
	CapFileList     Capability = "file.list"
	CapHTTPGet      Capability = "http.get"
	CapHTTPPost     Capability = "http.post"
	CapProviderCall Capability = "provider.call"
	// CapDelegate gates the `delegate` tool (spawn a sub-agent, P6-MULTI-01).
	// The delegation itself is allowed by default — it has no external side
	// effect of its own; the sub-agent's actual tool calls are each gated
	// through this same engine, so safety is enforced where the action happens.
	CapDelegate Capability = "delegate"
	// CapCoding gates the `coding` tool (delegate to an external coding agent
	// in an isolated git worktree, P6-CODE). It runs an external process that
	// writes files, so it is Ask-first by default — but the change lands only
	// in a throwaway worktree and is returned as a diff, never merged.
	CapCoding Capability = "coding"
	// CapACPAgent gates the `acp_agent` tool (delegate to an external agent over
	// the Agent Client Protocol, SPEC-15 §3). It spawns an external agent that
	// can act in its own sandbox, so it is Ask-first by default like coding.
	CapACPAgent Capability = "acp_agent"
	// CapRemoteRun gates the `remote_run` tool (delegate a task to a peer Agezt
	// node over its REST API, M8 mesh). It ships a task to an external node — an
	// outward, side-effecting action — so it is Ask-first by default.
	CapRemoteRun Capability = "remote_run"
)

// TrustLevel encodes the trust ladder (DECISIONS F3).
type TrustLevel int

const (
	// LevelDeny — hard block (L0). Never overridable per-cap.
	LevelDeny TrustLevel = 0
	// LevelAsk — every call requires approval (L1).
	LevelAsk TrustLevel = 1
	// LevelAskFirst — ask on first use per session (L2).
	LevelAskFirst TrustLevel = 2
	// LevelAskScoped — ask once per (session, scope) (L3).
	LevelAskScoped TrustLevel = 3
	// LevelAllow — silent allow (L4).
	LevelAllow TrustLevel = 4
)

// String returns the conventional Lx label.
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

// AskPolicy controls how the engine handles Ask-class levels (L1..L3)
// while live approval routing is not yet wired (lands in MVP).
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

// HardDenyRule is a single hard-deny pattern.
type HardDenyRule struct {
	Name      string       // human label, included in the Outcome
	Substring string       // case-insensitive substring match against input
	AppliesTo []Capability // empty = applies to every capability
}

// matches reports whether r fires against the input under the named cap.
func (r HardDenyRule) matches(cap Capability, input string) bool {
	if len(r.AppliesTo) > 0 && !slices.Contains(r.AppliesTo, cap) {
		return false
	}
	return strings.Contains(strings.ToLower(input), strings.ToLower(r.Substring))
}

// Options seed a new Engine.
type Options struct {
	// Levels overrides per-capability defaults. Caps absent here use the
	// values from Defaults (or LevelDeny if absent entirely).
	Levels map[Capability]TrustLevel
	// HardDeny replaces the default hard-deny rule set (use append with
	// DefaultHardDeny to extend rather than replace).
	HardDeny []HardDenyRule
	// AskPolicy chooses how to fold L1..L3. Default: AskAllow.
	AskPolicy AskPolicy
}

// Engine is the policy decision engine. Safe for concurrent use.
type Engine struct {
	mu        sync.RWMutex
	levels    map[Capability]TrustLevel
	hardDeny  []HardDenyRule
	askPolicy AskPolicy
}

// Levels returns a snapshot of the per-capability trust levels.
// Returned map is a copy — mutating it doesn't affect the engine.
// Used by the control plane to power `agt edict show`.
func (e *Engine) Levels() map[Capability]TrustLevel {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make(map[Capability]TrustLevel, len(e.levels))
	maps.Copy(out, e.levels)
	return out
}

// HardDenyRules returns a snapshot of the hard-deny rule set.
// Returned slice is a copy — same rationale as Levels().
func (e *Engine) HardDenyRules() []HardDenyRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]HardDenyRule, len(e.hardDeny))
	copy(out, e.hardDeny)
	return out
}

// AskPolicy returns the configured AskPolicy. Useful for the
// control plane's `agt edict show` so operators can confirm
// whether the daemon is currently in allow/deny/prompt mode.
func (e *Engine) AskPolicy() AskPolicy {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.askPolicy
}

// New builds an Engine. Unset Options fall back to DefaultLevels +
// DefaultHardDeny + AskAllow.
func New(opt Options) *Engine {
	defaults := DefaultLevels()
	levels := make(map[Capability]TrustLevel, len(defaults)+len(opt.Levels))
	maps.Copy(levels, defaults)
	maps.Copy(levels, opt.Levels)
	hd := opt.HardDeny
	if hd == nil {
		hd = DefaultHardDeny()
	}
	return &Engine{
		levels:    levels,
		hardDeny:  hd,
		askPolicy: opt.AskPolicy,
	}
}

// DefaultLevels are the per-capability defaults from DECISIONS F3, adapted
// to the M1 capability set. Approval routing isn't wired yet, so anything
// at L1..L3 will be folded by AskPolicy (default AskAllow + journal note).
func DefaultLevels() map[Capability]TrustLevel {
	return map[Capability]TrustLevel{
		CapShell:        LevelAskFirst, // L2 per F3
		CapFileRead:     LevelAllow,    // very common, low risk
		CapFileList:     LevelAllow,
		CapFileWrite:    LevelAskFirst, // L2
		CapFileDelete:   LevelAsk,      // L1 — always ask
		CapHTTPGet:      LevelAskFirst, // L2 (more permissive than F3's L1)
		CapHTTPPost:     LevelAsk,      // L1
		CapProviderCall: LevelAllow,    // governed by budget, not Edict
		CapDelegate:     LevelAllow,    // sub-agent spawn; its tool calls are gated individually
		CapCoding:       LevelAskFirst, // external coding agent; isolated to a worktree, returns a diff
		CapACPAgent:     LevelAskFirst, // external ACP agent; runs in its own sandbox, returns its answer
		CapRemoteRun:    LevelAskFirst, // peer Agezt node; ships a task to an external node over REST
	}
}

// DefaultHardDeny is the immutable hard-deny set from DECISIONS F4. The
// substrings are case-insensitive and only checked for the specified
// capabilities so they don't false-positive on unrelated tool input.
func DefaultHardDeny() []HardDenyRule {
	return []HardDenyRule{
		{Name: "fork-bomb", Substring: ":(){:|:&};:", AppliesTo: []Capability{CapShell}},
		{Name: "rm-rf-root", Substring: "rm -rf /", AppliesTo: []Capability{CapShell}},
		{Name: "rm-rf-root-flag", Substring: "rm -rf --no-preserve-root", AppliesTo: []Capability{CapShell}},
		{Name: "mkfs", Substring: "mkfs", AppliesTo: []Capability{CapShell}},
		{Name: "dd-of-dev", Substring: "dd if=", AppliesTo: []Capability{CapShell}}, // dd ... of=/dev/sda*
		{Name: "shutdown", Substring: "shutdown -", AppliesTo: []Capability{CapShell}},
		{Name: "reboot", Substring: "reboot", AppliesTo: []Capability{CapShell}},
		{Name: "powershell-format", Substring: "format-volume", AppliesTo: []Capability{CapShell}},
	}
}

// Decide returns the engine's verdict for one (capability, input) pair.
// input is the stringified tool input — the engine treats it as opaque
// text for hard-deny substring matching only. The runtime is responsible
// for journaling the Outcome.
func (e *Engine) Decide(cap Capability, input string) Outcome {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. Hard-deny always wins.
	for _, r := range e.hardDeny {
		if r.matches(cap, input) {
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

	// 2. Look up the trust level for this capability.
	lvl, ok := e.levels[cap]
	if !ok {
		// Unknown capability: default-deny (the strict reading of B0c +
		// secure-defaults). The user must explicitly grant a level via
		// Options.Levels to use a new capability.
		return Outcome{
			Decision:   DecisionDeny,
			Capability: cap,
			Level:      LevelDeny,
			Reason:     fmt.Sprintf("no trust level configured for %q (default-deny)", cap),
		}
	}

	// 3. Apply level → decision.
	switch lvl {
	case LevelDeny:
		return Outcome{
			Decision:   DecisionDeny,
			Capability: cap,
			Level:      lvl,
			Reason:     "capability set to L0 (deny)",
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
				Reason:     fmt.Sprintf("level %s requires approval; AskPolicy=AskDeny", lvl),
			}
		case AskPrompt:
			return Outcome{
				// Fail-closed default for callers that don't honour
				// RequiresApproval — the runtime overrides this after
				// a real grant.
				Decision:         DecisionDeny,
				Capability:       cap,
				Level:            lvl,
				Reason:           fmt.Sprintf("level %s; AskPolicy=AskPrompt → operator approval required", lvl),
				WouldAsk:         true,
				RequiresApproval: true,
			}
		default: // AskAllow
			return Outcome{
				Decision:   DecisionAllow,
				Capability: cap,
				Level:      lvl,
				Reason:     fmt.Sprintf("level %s; AskPolicy=AskAllow (would prompt in MVP)", lvl),
				WouldAsk:   true,
			}
		}
	}
}

// SetLevel changes the trust level for a capability at runtime. Useful
// for the future `agt trust` CLI; safe for concurrent use.
func (e *Engine) SetLevel(cap Capability, lvl TrustLevel) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.levels[cap] = lvl
}

// Level returns the current trust level for a capability, and a bool
// indicating whether it was explicitly configured (vs. default-deny).
func (e *Engine) Level(cap Capability) (TrustLevel, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	lvl, ok := e.levels[cap]
	return lvl, ok
}
