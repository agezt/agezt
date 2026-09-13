// SPDX-License-Identifier: MIT

// Package edict: Engine getters (Levels + HardDenyRules + AskPolicy +
// SetAskPolicy), New constructor, and Engine mutators (SetLevel + AddHardDeny
// + RemoveHardDeny). Default levels/rules/parsing moved to edict_defaults.go;
// Decide + DecideWithCeiling moved to edict_decide.go. Day-211 god-file split.
// Public API unchanged.
package edict


import (
	"fmt"
	"maps"
	"slices"
	"strings"
)
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

// SetAskPolicy changes the engine-wide approval mode at runtime. The
// hard-deny floor is unaffected (it fires before AskPolicy is consulted),
// so even AskAllow can't relax a hard-deny. Safe for concurrent use.
func (e *Engine) SetAskPolicy(p AskPolicy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.askPolicy = p
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
		levels:       levels,
		hardDeny:     hd,
		askPolicy:    opt.AskPolicy,
		unknownAllow: opt.UnknownAllow,
	}
}
func (e *Engine) SetLevel(cap Capability, lvl TrustLevel) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.levels[cap] = lvl
}

// AddHardDeny appends a hard-deny rule at runtime and returns the stored
// rule (with its engine-assigned name). The caller supplies Substring and
// AppliesTo (e.g. from ParseDenyRules); the Name is always overwritten
// with a fresh "runtime[N]" so the rule is removable via RemoveHardDeny
// and can never be confused with a built-in or operator[N] floor rule.
//
// A blank substring is rejected for the same reason ParseDenyRules rejects
// it: a rule matching the empty string would deny every action. Safe for
// concurrent use; the change takes effect on the next Decide.
func (e *Engine) AddHardDeny(rule HardDenyRule) (HardDenyRule, error) {
	if strings.TrimSpace(rule.Substring) == "" {
		return HardDenyRule{}, fmt.Errorf("edict: hard-deny rule has an empty substring (would deny everything)")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rtSeq++
	rule.Name = fmt.Sprintf("%s%d]", RuntimeRulePrefix, e.rtSeq)
	e.hardDeny = append(e.hardDeny, rule)
	return rule, nil
}

// RemoveHardDeny removes the runtime-added hard-deny rule named name and
// reports whether a rule was removed. It refuses to touch the boot-time
// floor: removing a built-in or operator[N] rule returns an error, never a
// silent success — the floor stays put. Safe for concurrent use.
func (e *Engine) RemoveHardDeny(name string) (bool, error) {
	if !IsRuntimeRule(name) {
		return false, fmt.Errorf("edict: %q is not a runtime-added rule; the boot-time deny floor cannot be removed at runtime", name)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, r := range e.hardDeny {
		if r.Name == name {
			e.hardDeny = slices.Delete(e.hardDeny, i, i+1)
			return true, nil
		}
	}
	return false, nil
}
