// SPDX-License-Identifier: MIT

// Edict engine: policy overlay types + projection + apply.
// Code extracted from edict_engine.go during the Day-87 god-file split.
// Public API unchanged.
package edict


import (
	"slices"
	"strings"
)

type PolicyChange struct {
	Action     string   `json:"action"`
	Capability string   `json:"capability"`
	To         string   `json:"to"`
	Name       string   `json:"name"`
	Substring  string   `json:"substring"`
	AppliesTo  []string `json:"applies_to"`
}

// PolicyOverlay is the net effect of a sequence of PolicyChanges: the
// per-capability level overrides plus the surviving runtime deny rules.
// It is what a daemon replays onto a freshly-booted engine to make
// runtime policy changes durable across a restart.
type PolicyOverlay struct {
	Levels    map[Capability]TrustLevel
	DenyRules []HardDenyRule
	// Mode, when non-nil, is the net runtime approval-mode override. A
	// pointer so "no mode change in history" is distinct from AskAllow (0).
	Mode *AskPolicy
}

// IsEmpty reports whether the overlay carries no changes — lets a caller
// skip the apply (and a banner line) when there's nothing to restore.
func (o PolicyOverlay) IsEmpty() bool {
	return len(o.Levels) == 0 && len(o.DenyRules) == 0 && o.Mode == nil
}

// ProjectPolicyChanges folds an ordered sequence of policy.changed events
// into the net overlay. It is pure (no engine, no I/O) so it is trivially
// testable and the daemon's only job is to decode the journal and apply
// the result.
//
// Semantics: level.set is last-wins per capability; deny.add/deny.rm are
// tracked by the rule's journaled name so an add later removed leaves no
// trace, and surviving rules keep their original add order. Malformed
// entries (blank capability/substring, unparseable level) are skipped
// rather than failing the whole replay — one bad historical event must
// not wedge a restart.
func ProjectPolicyChanges(changes []PolicyChange) PolicyOverlay {
	levels := map[Capability]TrustLevel{}
	denyByName := map[string]HardDenyRule{}
	var order []string // surviving deny names, in add order
	var mode *AskPolicy
	for _, ch := range changes {
		switch ch.Action {
		case "mode.set":
			if p, err := ParseAskPolicy(ch.To); err == nil {
				mode = &p // last-wins
			}
		case "level.set":
			if ch.Capability == "" {
				continue
			}
			lvl, err := ParseTrustLevel(ch.To)
			if err != nil {
				continue
			}
			levels[Capability(ch.Capability)] = lvl
		case "deny.add":
			if ch.Name == "" || strings.TrimSpace(ch.Substring) == "" {
				continue
			}
			var caps []Capability
			for _, c := range ch.AppliesTo {
				if c != "" {
					caps = append(caps, Capability(c))
				}
			}
			if _, seen := denyByName[ch.Name]; !seen {
				order = append(order, ch.Name)
			}
			denyByName[ch.Name] = HardDenyRule{Name: ch.Name, Substring: ch.Substring, AppliesTo: caps}
		case "deny.rm":
			if _, ok := denyByName[ch.Name]; ok {
				delete(denyByName, ch.Name)
				for i, n := range order {
					if n == ch.Name {
						order = slices.Delete(order, i, i+1)
						break
					}
				}
			}
		}
	}
	overlay := PolicyOverlay{Mode: mode}
	if len(levels) > 0 {
		overlay.Levels = levels
	}
	for _, n := range order {
		overlay.DenyRules = append(overlay.DenyRules, denyByName[n])
	}
	return overlay
}

// ApplyOverlay applies a projected overlay onto the engine: level
// overrides via SetLevel, surviving runtime deny rules via AddHardDeny
// (each re-assigned a fresh runtime[N] name). Returns the counts applied,
// for an operator banner. Used by the daemon when AGEZT_EDICT_DURABLE is
// on to restore runtime policy from the journal.
func (e *Engine) ApplyOverlay(o PolicyOverlay) (levels, rules int) {
	if o.Mode != nil {
		e.SetAskPolicy(*o.Mode)
	}
	for cap, lvl := range o.Levels {
		e.SetLevel(cap, lvl)
		levels++
	}
	for _, r := range o.DenyRules {
		if _, err := e.AddHardDeny(r); err == nil {
			rules++
		}
	}
	return levels, rules
}

// Level returns the current trust level for a capability, and a bool
// indicating whether it was explicitly configured (vs. default-deny).
func (e *Engine) Level(cap Capability) (TrustLevel, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	lvl, ok := e.levels[cap]
	return lvl, ok
}

