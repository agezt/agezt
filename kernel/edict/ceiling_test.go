// SPDX-License-Identifier: MIT

package edict

import "testing"

// TestDecideWithCeiling: the per-call trust ceiling clamps an auto-allowed
// capability down to Ask (or Deny), never loosens, and never bypasses hard-deny
// (SPEC-16 §4 initiative.max_trust / M408).
func TestDecideWithCeiling(t *testing.T) {
	e := New(Options{
		Levels:    map[Capability]TrustLevel{CapShell: LevelAllow}, // L4: normally auto-allow
		AskPolicy: AskAllow,
	})

	// No ceiling (LevelAllow) → unchanged: allow.
	if o := e.DecideWithCeiling(CapShell, "echo hi", LevelAllow); o.Decision != DecisionAllow {
		t.Errorf("ceiling L4 should not clamp an L4 cap: got %s", o.Decision)
	}
	// Ceiling L2 → clamps L4 down into the Ask band (AskAllow → allow-with-WouldAsk).
	o := e.DecideWithCeiling(CapShell, "echo hi", LevelAskFirst)
	if o.Level != LevelAskFirst || !o.WouldAsk {
		t.Errorf("ceiling L2 should clamp to L2/WouldAsk, got level=%s wouldAsk=%v", o.Level, o.WouldAsk)
	}
	// Ceiling L0 → deny outright.
	if o := e.DecideWithCeiling(CapShell, "echo hi", LevelDeny); o.Decision != DecisionDeny {
		t.Errorf("ceiling L0 should deny, got %s", o.Decision)
	}
	// A ceiling never LOOSENS: a deny-configured cap stays denied even at ceiling L4.
	e2 := New(Options{Levels: map[Capability]TrustLevel{CapShell: LevelDeny}})
	if o := e2.DecideWithCeiling(CapShell, "echo hi", LevelAllow); o.Decision != DecisionDeny {
		t.Errorf("ceiling must not loosen an L0 cap, got %s", o.Decision)
	}
	// Hard-deny is never bypassed by a high ceiling.
	e3 := New(Options{Levels: map[Capability]TrustLevel{CapShell: LevelAllow}})
	if o := e3.DecideWithCeiling(CapShell, "rm -rf /", LevelAllow); !o.HardDenied {
		t.Errorf("hard-deny must hold regardless of ceiling, got %+v", o)
	}
	// Decide is exactly DecideWithCeiling at LevelAllow (no behaviour change).
	if e.Decide(CapShell, "echo hi").Decision != e.DecideWithCeiling(CapShell, "echo hi", LevelAllow).Decision {
		t.Error("Decide should equal DecideWithCeiling(LevelAllow)")
	}
}
