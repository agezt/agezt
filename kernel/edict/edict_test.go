// SPDX-License-Identifier: MIT

package edict

import (
	"strings"
	"testing"
)

func TestParseDenyRules(t *testing.T) {
	rules, err := ParseDenyRules("git push ; shell:rm -rf /etc ; https://evil.example ; http.post:169.254 ;  ; file.delete:/etc")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rules) != 5 { // the blank entry is skipped
		t.Fatalf("got %d rules, want 5: %+v", len(rules), rules)
	}
	// "git push" — no known-cap prefix → applies to all caps.
	if rules[0].Substring != "git push" || len(rules[0].AppliesTo) != 0 {
		t.Errorf("rule0 = %+v", rules[0])
	}
	// "shell:rm -rf /etc" — cap-scoped to shell.
	if rules[1].Substring != "rm -rf /etc" || len(rules[1].AppliesTo) != 1 || rules[1].AppliesTo[0] != CapShell {
		t.Errorf("rule1 = %+v", rules[1])
	}
	// "https://evil.example" — prefix "https" is NOT a capability → verbatim, all caps.
	if rules[2].Substring != "https://evil.example" || len(rules[2].AppliesTo) != 0 {
		t.Errorf("rule2 = %+v (https prefix must not be parsed as a capability)", rules[2])
	}
	// "http.post:169.254" — cap-scoped.
	if rules[3].AppliesTo[0] != CapHTTPPost || rules[3].Substring != "169.254" {
		t.Errorf("rule3 = %+v", rules[3])
	}
	if rules[4].AppliesTo[0] != CapFileDelete || rules[4].Substring != "/etc" {
		t.Errorf("rule4 = %+v", rules[4])
	}
}

func TestParseDenyRules_RejectsEmptySubstring(t *testing.T) {
	// A bare "shell:" (known cap, empty substring) would deny ALL shell — reject.
	if _, err := ParseDenyRules("shell:"); err == nil {
		t.Error("expected an error for an empty cap-scoped substring")
	}
	// Empty / whitespace-only spec is a no-op, not an error.
	if rules, err := ParseDenyRules("   "); err != nil || rules != nil {
		t.Errorf("blank spec: rules=%v err=%v, want nil,nil", rules, err)
	}
}

func TestParsedDenyRules_FireThroughDecide(t *testing.T) {
	extra, err := ParseDenyRules("git push ; shell:/etc/shadow")
	if err != nil {
		t.Fatal(err)
	}
	e := New(Options{
		Levels:   map[Capability]TrustLevel{CapShell: LevelAllow, CapHTTPPost: LevelAllow},
		HardDeny: append(DefaultHardDeny(), extra...),
	})
	// "git push" denied for any capability (all-caps rule), even though shell=L4.
	if o := e.Decide(CapShell, "git push origin main"); !o.HardDenied || o.Decision != DecisionDeny {
		t.Errorf("git push should be hard-denied: %+v", o)
	}
	// The cap-scoped rule fires for shell...
	if o := e.Decide(CapShell, "cat /etc/shadow"); !o.HardDenied {
		t.Errorf("/etc/shadow should be hard-denied on shell: %+v", o)
	}
	// ...but NOT for a different capability (http.post), which the rule doesn't target.
	if o := e.Decide(CapHTTPPost, "POST body mentioning /etc/shadow"); o.HardDenied {
		t.Errorf("cap-scoped shell rule must not fire on http.post: %+v", o)
	}
	// The built-in rules still work alongside the custom ones.
	if o := e.Decide(CapShell, "rm -rf /"); !o.HardDenied {
		t.Errorf("built-in rm-rf rule should still fire: %+v", o)
	}
	// An ordinary command is allowed (shell=L4).
	if o := e.Decide(CapShell, "echo hello"); o.Decision != DecisionAllow {
		t.Errorf("ordinary command should be allowed: %+v", o)
	}
}

func TestAddHardDeny_AssignsRuntimeNameAndFires(t *testing.T) {
	e := New(Options{Levels: map[Capability]TrustLevel{CapShell: LevelAllow}})
	r, err := e.AddHardDeny(HardDenyRule{Substring: "kubectl delete", AppliesTo: []Capability{CapShell}})
	if err != nil {
		t.Fatalf("AddHardDeny: %v", err)
	}
	// Name is engine-assigned with the runtime prefix, regardless of any
	// supplied name — that's the invariant RemoveHardDeny relies on.
	if !IsRuntimeRule(r.Name) {
		t.Errorf("added rule name %q lacks runtime prefix", r.Name)
	}
	if o := e.Decide(CapShell, "kubectl delete pod x"); !o.HardDenied {
		t.Errorf("runtime-added rule should hard-deny: %+v", o)
	}
	// The supplied Name is overwritten, not honoured.
	r2, _ := e.AddHardDeny(HardDenyRule{Name: "forged", Substring: "rm secret"})
	if r2.Name == "forged" {
		t.Errorf("AddHardDeny must overwrite the caller's name; got %q", r2.Name)
	}
	if r2.Name == r.Name {
		t.Errorf("runtime names must be unique; both = %q", r.Name)
	}
}

func TestAddHardDeny_RejectsEmptySubstring(t *testing.T) {
	e := New(Options{})
	if _, err := e.AddHardDeny(HardDenyRule{Substring: "   "}); err == nil {
		t.Error("empty/whitespace substring must be rejected")
	}
}

func TestRemoveHardDeny_OnlyRuntimeRules(t *testing.T) {
	e := New(Options{Levels: map[Capability]TrustLevel{CapShell: LevelAllow}})
	added, _ := e.AddHardDeny(HardDenyRule{Substring: "kubectl delete", AppliesTo: []Capability{CapShell}})

	// A built-in floor rule cannot be removed.
	if _, err := e.RemoveHardDeny("rm-rf-root"); err == nil {
		t.Error("removing a built-in rule must error")
	}
	// An operator[N] (AGEZT_EDICT_DENY) rule cannot be removed either.
	if _, err := e.RemoveHardDeny("operator[1]"); err == nil {
		t.Error("removing an operator[N] floor rule must error")
	}
	// The built-in still fires (it was never touched).
	if o := e.Decide(CapShell, "rm -rf /"); !o.HardDenied {
		t.Error("built-in rule must survive a failed remove attempt")
	}

	// The runtime rule removes cleanly.
	removed, err := e.RemoveHardDeny(added.Name)
	if err != nil || !removed {
		t.Fatalf("RemoveHardDeny(%q) = %v, %v; want true, nil", added.Name, removed, err)
	}
	if o := e.Decide(CapShell, "kubectl delete pod x"); o.HardDenied {
		t.Error("removed runtime rule must stop firing")
	}
	// Removing an unknown-but-runtime-shaped name is a clean false, no error.
	if removed, err := e.RemoveHardDeny("runtime[999]"); err != nil || removed {
		t.Errorf("removing absent runtime rule = %v, %v; want false, nil", removed, err)
	}
}

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
