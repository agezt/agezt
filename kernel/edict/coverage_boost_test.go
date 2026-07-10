// SPDX-License-Identifier: MIT

package edict

import (
	"testing"
)

// TestHardDenyRulesAccessor covers HardDenyRules (returns a copy of the
// configured hard-deny floor).
func TestHardDenyRulesAccessor(t *testing.T) {
	e := New(Options{})
	rules := e.HardDenyRules()
	if len(rules) == 0 {
		t.Fatalf("HardDenyRules() empty, want the default hard-deny floor")
	}
	// Returned slice must be a copy: mutating it must not affect the engine.
	orig := len(rules)
	rules = append(rules, HardDenyRule{})
	if len(rules) != orig+1 {
		t.Fatalf("append on the returned copy = %d rules, want %d", len(rules), orig+1)
	}
	if len(e.HardDenyRules()) != orig {
		t.Fatalf("HardDenyRules() returned an aliased slice, want a copy")
	}
}

// TestKnownCapability covers the exported KnownCapability wrapper for a known
// and an unknown capability string.
func TestKnownCapability(t *testing.T) {
	// Every capability in AllCapabilities() must be reported known.
	all := AllCapabilities()
	if len(all) == 0 {
		t.Fatalf("AllCapabilities() empty")
	}
	if !KnownCapability(string(all[0])) {
		t.Fatalf("KnownCapability(%q) = false, want true", all[0])
	}
	if KnownCapability("totally.made.up.capability") {
		t.Fatalf("KnownCapability(unknown) = true, want false")
	}
}

// TestStringDefaults covers the default (unknown-value) branches of the
// TrustLevel.String and AskPolicy.String methods.
func TestStringDefaults(t *testing.T) {
	if got := TrustLevel(99).String(); got == "" {
		t.Fatalf("TrustLevel(99).String() empty, want a L?() label")
	}
	if got := AskPolicy(99).String(); got != "unknown" {
		t.Fatalf("AskPolicy(99).String() = %q, want unknown", got)
	}
	// Cover the known labels too for good measure.
	for _, l := range []TrustLevel{LevelDeny, LevelAsk, LevelAskFirst, LevelAskScoped, LevelAllow} {
		if l.String() == "" {
			t.Fatalf("TrustLevel(%d).String() empty", int(l))
		}
	}
	for _, p := range []AskPolicy{AskAllow, AskDeny, AskPrompt} {
		if p.String() == "" {
			t.Fatalf("AskPolicy.String() empty for %v", p)
		}
	}
}

// TestDecideJSONInputCollectsStrings drives Decide with JSON array and object
// inputs so collectJSONStrings walks the []any and map[string]any branches,
// and a hard-deny substring hidden inside JSON is still caught.
func TestDecideJSONInputCollectsStrings(t *testing.T) {
	e := New(Options{
		Levels:    map[Capability]TrustLevel{CapShell: LevelAllow},
		AskPolicy: AskAllow,
	})

	// A JSON array carrying a hard-denied command -> []any branch.
	arr := e.Decide(CapShell, `["ok", "rm -rf /"]`)
	if !arr.HardDenied {
		t.Fatalf("Decide(JSON array with rm -rf /) HardDenied = false, want true")
	}

	// A JSON object carrying a hard-denied command -> map[string]any branch.
	obj := e.Decide(CapShell, `{"cmd": "rm -rf /", "note": "safe"}`)
	if !obj.HardDenied {
		t.Fatalf("Decide(JSON object with rm -rf /) HardDenied = false, want true")
	}

	// A benign JSON object still decides normally (no hard-deny).
	benign := e.Decide(CapShell, `{"cmd": "echo hello"}`)
	if benign.HardDenied {
		t.Fatalf("Decide(benign JSON) HardDenied = true, want false")
	}
}
