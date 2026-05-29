// SPDX-License-Identifier: MIT

package edict

import (
	"strings"
	"testing"
)

func TestDefaultLevels_RespectF3(t *testing.T) {
	e := New(Options{})
	checks := map[Capability]TrustLevel{
		CapShell:      LevelAskFirst,
		CapFileRead:   LevelAllow,
		CapFileWrite:  LevelAskFirst,
		CapFileDelete: LevelAsk,
		CapHTTPGet:    LevelAskFirst,
		CapHTTPPost:   LevelAsk,
	}
	for cap, want := range checks {
		got, ok := e.Level(cap)
		if !ok {
			t.Errorf("%s missing from default levels", cap)
			continue
		}
		if got != want {
			t.Errorf("%s: level=%s want %s", cap, got, want)
		}
	}
}

func TestDecide_AllowL4(t *testing.T) {
	e := New(Options{Levels: map[Capability]TrustLevel{CapShell: LevelAllow}})
	o := e.Decide(CapShell, "echo hi")
	if o.Decision != DecisionAllow {
		t.Errorf("got %v want allow", o)
	}
	if o.WouldAsk {
		t.Error("L4 should not set WouldAsk")
	}
}

func TestDecide_DenyL0(t *testing.T) {
	e := New(Options{Levels: map[Capability]TrustLevel{CapShell: LevelDeny}})
	o := e.Decide(CapShell, "echo hi")
	if o.Decision != DecisionDeny {
		t.Errorf("got %v want deny", o)
	}
}

func TestDecide_AskFoldsToAllowByDefault(t *testing.T) {
	e := New(Options{Levels: map[Capability]TrustLevel{CapShell: LevelAskFirst}})
	o := e.Decide(CapShell, "echo hi")
	if o.Decision != DecisionAllow {
		t.Errorf("AskAllow should fold L2 → allow; got %v", o)
	}
	if !o.WouldAsk {
		t.Error("WouldAsk should be set on folded ask")
	}
}

func TestDecide_AskDenyMode(t *testing.T) {
	e := New(Options{
		Levels:    map[Capability]TrustLevel{CapShell: LevelAskFirst},
		AskPolicy: AskDeny,
	})
	o := e.Decide(CapShell, "echo hi")
	if o.Decision != DecisionDeny {
		t.Errorf("AskDeny should reject L2; got %v", o)
	}
}

func TestDecide_AskPromptMarksRequiresApproval(t *testing.T) {
	e := New(Options{
		Levels:    map[Capability]TrustLevel{CapShell: LevelAskFirst, CapFileDelete: LevelAsk},
		AskPolicy: AskPrompt,
	})
	for _, cap := range []Capability{CapShell, CapFileDelete} {
		o := e.Decide(cap, "anything")
		if !o.RequiresApproval {
			t.Errorf("%s: RequiresApproval=false; want true under AskPrompt", cap)
		}
		if o.Decision != DecisionDeny {
			t.Errorf("%s: Decision=%v want Deny (fail-closed default under AskPrompt)", cap, o.Decision)
		}
		if !o.WouldAsk {
			t.Errorf("%s: WouldAsk should remain true under AskPrompt", cap)
		}
	}
}

func TestDecide_AskPromptDoesNotBypassHardDeny(t *testing.T) {
	e := New(Options{
		Levels:    map[Capability]TrustLevel{CapShell: LevelAskFirst},
		AskPolicy: AskPrompt,
	})
	o := e.Decide(CapShell, "rm -rf /")
	if !o.HardDenied {
		t.Error("hard-deny should fire even under AskPrompt")
	}
	if o.RequiresApproval {
		t.Error("hard-deny must NOT request approval")
	}
}

func TestDecide_HardDenyAlwaysWins(t *testing.T) {
	// Even at L4 a hard-deny match must block.
	e := New(Options{Levels: map[Capability]TrustLevel{CapShell: LevelAllow}})
	o := e.Decide(CapShell, "sudo rm -rf / --no-preserve-root")
	if o.Decision != DecisionAllow {
		// the "rm -rf /" substring should match
	}
	o = e.Decide(CapShell, "rm -rf /")
	if o.Decision != DecisionDeny {
		t.Errorf("rm -rf / should be hard-denied; got %v", o)
	}
	if !o.HardDenied || o.HardDenyRule == "" {
		t.Errorf("HardDenied/HardDenyRule not set: %v", o)
	}
}

func TestDecide_HardDenyScopedToCapability(t *testing.T) {
	// "rm -rf /" in an http body should NOT be hard-denied (rule scopes
	// to CapShell). Verifies AppliesTo works.
	e := New(Options{Levels: map[Capability]TrustLevel{CapHTTPPost: LevelAllow}})
	o := e.Decide(CapHTTPPost, `{"body":"rm -rf /"}`)
	if o.Decision != DecisionAllow {
		t.Errorf("http POST should be allowed despite payload containing shell-only hard-deny pattern; got %v", o)
	}
}

func TestDecide_UnknownCapability_DefaultDeny(t *testing.T) {
	e := New(Options{})
	o := e.Decide(Capability("unknown.cap"), "any input")
	if o.Decision != DecisionDeny {
		t.Errorf("unknown capability must default-deny; got %v", o)
	}
	if !strings.Contains(o.Reason, "default-deny") {
		t.Errorf("reason should mention default-deny; got %q", o.Reason)
	}
}

func TestDecide_ForkBombDenied(t *testing.T) {
	e := New(Options{Levels: map[Capability]TrustLevel{CapShell: LevelAllow}})
	o := e.Decide(CapShell, "echo classic ; :(){:|:&};: ; echo end")
	if o.Decision != DecisionDeny || !o.HardDenied {
		t.Errorf("fork bomb should be hard-denied; got %v", o)
	}
}

func TestSetLevel(t *testing.T) {
	e := New(Options{})
	e.SetLevel(CapShell, LevelDeny)
	o := e.Decide(CapShell, "echo")
	if o.Decision != DecisionDeny {
		t.Errorf("SetLevel did not take effect: %v", o)
	}
}

func TestTrustLevel_String(t *testing.T) {
	cases := map[TrustLevel]string{
		LevelDeny: "L0", LevelAsk: "L1", LevelAskFirst: "L2", LevelAskScoped: "L3", LevelAllow: "L4",
	}
	for l, want := range cases {
		if got := l.String(); got != want {
			t.Errorf("(%d).String()=%q want %q", l, got, want)
		}
	}
}

func TestHardDeny_CaseInsensitive(t *testing.T) {
	e := New(Options{Levels: map[Capability]TrustLevel{CapShell: LevelAllow}})
	for _, cmd := range []string{"RM -RF /", "Rm -Rf /", "rm -rf /"} {
		o := e.Decide(CapShell, cmd)
		if o.Decision != DecisionDeny {
			t.Errorf("case-insensitive match failed for %q: %v", cmd, o)
		}
	}
}

func TestCustomHardDenyList(t *testing.T) {
	e := New(Options{
		Levels:   map[Capability]TrustLevel{CapShell: LevelAllow},
		HardDeny: []HardDenyRule{{Name: "no-curl", Substring: "curl evil.com"}},
	})
	if o := e.Decide(CapShell, "echo hi"); o.Decision != DecisionAllow {
		t.Errorf("custom list should not block unrelated input: %v", o)
	}
	o := e.Decide(CapShell, "curl evil.com/x")
	if o.Decision != DecisionDeny || o.HardDenyRule != "no-curl" {
		t.Errorf("custom rule should fire: %v", o)
	}
}
