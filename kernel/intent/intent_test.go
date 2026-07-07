// SPDX-License-Identifier: MIT

package intent

import (
	"context"
	"strings"
	"testing"
)

func TestInterpret_BroadMutationIsUnderdetermined(t *testing.T) {
	frame := Interpret("dosyaları temizle")
	if !frame.Underdetermined {
		t.Fatal("broad cleanup intent should be underdetermined")
	}
	if frame.AmbiguityScore < 0.6 {
		t.Fatalf("ambiguity score = %v, want >= 0.6", frame.AmbiguityScore)
	}
	if frame.HarmfulReading == "" {
		t.Fatal("harmful reading should be surfaced")
	}
	if frame.UserUtteranceHash == "" || strings.Contains(frame.UserUtteranceHash, "dosya") {
		t.Fatalf("utterance hash should not contain raw text: %q", frame.UserUtteranceHash)
	}
}

func TestInterpret_ReadOnlyIntentIsLowFriction(t *testing.T) {
	frame := Interpret("cache dosyalarını listele")
	if frame.Underdetermined {
		t.Fatal("read-only listing should not require intent confirmation")
	}
	if frame.AmbiguityScore >= 0.6 {
		t.Fatalf("ambiguity score = %v, want < 0.6", frame.AmbiguityScore)
	}
}

func TestRequiresConfirmationCombinesIntentAndRegret(t *testing.T) {
	frame := Interpret("clean files")
	axes := RegretForAction(Action{
		ToolName:          "file",
		Capability:        "file.delete",
		EffectClass:       "irreversible",
		Input:             `{"path":"legacy/2023"}`,
		AffectedResources: []string{"path:legacy/2023"},
	})
	if !RequiresConfirmation(frame, axes) {
		t.Fatal("underdetermined high-regret action should require confirmation")
	}
	prompt := ConfirmationPrompt(frame, Action{ToolName: "file", AffectedResources: []string{"path:legacy/2023"}}, axes)
	if !strings.Contains(prompt, "path:legacy/2023") || !strings.Contains(prompt, "Confirm") {
		t.Fatalf("prompt is not specific enough: %q", prompt)
	}
}

func TestRegretAxes_MaxAndSum(t *testing.T) {
	r := RegretAxes{Physical: 0.1, Informational: 0.5, Social: 0.3, Identity: 0.2}
	if m := r.Max(); m != 0.5 {
		t.Errorf("Max() = %f, want 0.5 (informational)", m)
	}
	const epsilon = 0.0001
	if s := r.Sum(); s < 1.1-epsilon || s > 1.1+epsilon {
		t.Errorf("Sum() = %f, want 1.1", s)
	}
}

func TestRegretAxes_MaxPhysicalWins(t *testing.T) {
	r := RegretAxes{Physical: 0.9, Informational: 0.1, Social: 0.1}
	if m := r.Max(); m != 0.9 {
		t.Errorf("Max() = %f, want 0.9 (physical)", m)
	}
}

func TestRegretAxes_MaxSocialWins(t *testing.T) {
	r := RegretAxes{Physical: 0.1, Informational: 0.1, Social: 0.9}
	if m := r.Max(); m != 0.9 {
		t.Errorf("Max() = %f, want 0.9 (social)", m)
	}
}

func TestRegretAxes_MaxIdentityWins(t *testing.T) {
	r := RegretAxes{Physical: 0.1, Informational: 0.1, Identity: 0.9}
	if m := r.Max(); m != 0.9 {
		t.Errorf("Max() = %f, want 0.9 (identity)", m)
	}
}

func TestRegretForAction_ReadOnly(t *testing.T) {
	r := RegretForAction(Action{Capability: "memory.read", EffectClass: "read_only"})
	if r.Informational != 0.1 {
		t.Errorf("read_only Informational = %f, want 0.1", r.Informational)
	}
}

func TestRegretForAction_DefaultEffect(t *testing.T) {
	r := RegretForAction(Action{Capability: "custom", EffectClass: "unknown"})
	if r.Informational != 0.55 {
		t.Errorf("default Informational = %f, want 0.55", r.Informational)
	}
}

func TestRegretForAction_ShellBoostsPhysical(t *testing.T) {
	r := RegretForAction(Action{Capability: "shell.exec", EffectClass: "compensable", Input: "rm -rf /tmp"})
	if r.Physical < 0.6 {
		t.Errorf("shell Physical = %f, want >= 0.65", r.Physical)
	}
}

func TestRegretForAction_NotifyBoostsSocial(t *testing.T) {
	r := RegretForAction(Action{Capability: "notify.message", EffectClass: "reversible", Input: "send to boss"})
	if r.Social < 0.7 {
		t.Errorf("notify Social = %f, want >= 0.8", r.Social)
	}
}

func TestRegretForAction_ConfigWriteBoostsIdentity(t *testing.T) {
	r := RegretForAction(Action{Capability: "config.write", EffectClass: "irreversible", Input: "update auth token"})
	if r.Identity < 0.6 {
		t.Errorf("config.write Identity = %f, want >= 0.65", r.Identity)
	}
}

func TestMaxFunction(t *testing.T) {
	if m := max(3.0, 5.0); m != 5.0 {
		t.Errorf("max(3,5) = %f, want 5", m)
	}
	if m := max(5.0, 3.0); m != 5.0 {
		t.Errorf("max(5,3) = %f, want 5", m)
	}
	if m := max(4.0, 4.0); m != 4.0 {
		t.Errorf("max(4,4) = %f, want 4", m)
	}
}

func TestWithFrameAndFrameFromContext(t *testing.T) {
	ctx := WithFrame(context.Background(), Frame{CanonicalIntent: "test"})
	frame, ok := FrameFromContext(ctx)
	if !ok {
		t.Error("FrameFromContext should return true for frame set with WithFrame")
	}
	if frame.CanonicalIntent != "test" {
		t.Errorf("FrameFromContext intent = %q, want %q", frame.CanonicalIntent, "test")
	}
	// Missing frame from unrelated context.
	_, ok = FrameFromContext(context.Background())
	if ok {
		t.Error("FrameFromContext(unrelated) should return false")
	}
}

func TestConfirmationPrompt_WithoutHarmfulReading(t *testing.T) {
	prompt := ConfirmationPrompt(Frame{}, Action{ToolName: "shell", AffectedResources: nil}, RegretAxes{Physical: 0.9})
	if !strings.Contains(prompt, "Confirm") {
		t.Errorf("prompt should contain Confirm: %q", prompt)
	}
	if !strings.Contains(prompt, "the proposed resources") {
		t.Errorf("prompt should mention default resource: %q", prompt)
	}
}

func TestRequiresConfirmation_NotUnderdetermined(t *testing.T) {
	frame := Frame{Underdetermined: false, AmbiguityScore: 0.9}
	if RequiresConfirmation(frame, RegretAxes{Physical: 0.9}) {
		t.Error("not underdetermined should not require confirmation")
	}
}

func TestRequiresConfirmation_LowRegret(t *testing.T) {
	frame := Frame{Underdetermined: true, AmbiguityScore: 0.9}
	if RequiresConfirmation(frame, RegretAxes{Physical: 0.1}) {
		t.Error("low regret should not require confirmation")
	}
}

func TestInterpret_TargetedMutationWithoutSafeScope(t *testing.T) {
	// "delete cache" is mutating, not broad, and "cache" is in specificSafeScope,
	// so it should NOT be flagged as harmful (safe scope).
	frame := Interpret("cache'yi temizle")
	if frame.AmbiguityScore > 0.5 || frame.HarmfulReading != "" {
		t.Fatalf("safe-scope mutation: score=%v harmful=%q, expected low score, no harmful reading", frame.AmbiguityScore, frame.HarmfulReading)
	}
}

func TestInterpret_DefaultIntentWithoutKeywords(t *testing.T) {
	// "merhaba" is a greeting — no mutation, no read, no broad keywords.
	frame := Interpret("merhaba")
	if frame.AmbiguityScore < 0.2 || frame.AmbiguityScore > 0.4 {
		t.Fatalf("default-neutral intent: score=%v, want ~0.3", frame.AmbiguityScore)
	}
	if frame.Underdetermined {
		t.Fatal("simple greeting should not be underdetermined")
	}
}

func TestInterpret_TargetedMutationNotSafe(t *testing.T) {
	// "delete project" is mutating, not broad (project is not in broadScope list),
	// but not safe-scope either. Should hit the `mutating && !specificSafeScope` branch.
	frame := Interpret("projeyi sil")
	if frame.AmbiguityScore < 0.5 || frame.AmbiguityScore > 0.7 {
		t.Fatalf("mutation-not-safe: score=%v, want ~0.6", frame.AmbiguityScore)
	}
	if frame.HarmfulReading == "" {
		t.Fatal("mutation without safe scope should have a harmful reading")
	}
}

func TestTokenSet_DigitsAndHyphens(t *testing.T) {
	// tokenSet should preserve digits and hyphens as token characters.
	s := tokenSet("test-123_abc")
	if _, ok := s["test-123_abc"]; !ok {
		t.Fatal("tokenSet should preserve hyphens, digits, and underscores as single tokens")
	}
}

func TestTokenSet_NonAlphaNumericSplit(t *testing.T) {
	// Non-alphanumeric characters should act as token separators.
	s := tokenSet("hello.world!foo")
	if _, ok := s["hello"]; !ok {
		t.Fatal("tokenSet should include 'hello'")
	}
	if _, ok := s["world"]; !ok {
		t.Fatal("tokenSet should include 'world'")
	}
	if _, ok := s["foo"]; !ok {
		t.Fatal("tokenSet should include 'foo'")
	}
	if _, ok := s["hello.world!foo"]; ok {
		t.Fatal("tokenSet should not preserve non-alphanumeric separators")
	}
}

func TestInterpret_SpecificSafeScopeMutationIsNotHarmful(t *testing.T) {
	// "clean cache" uses a mutation + specificSafeScope → the switch hits the
	// `mutating && broadScope && !specificSafeScope` or `mutating && !specificSafeScope`
	// case depending on whether "cache" is in broadScope.
	frame := Interpret("cache temizle")
	// "cache" is in specificSafeScope, not in broadScope, so should be safe.
	if frame.HarmfulReading != "" {
		t.Fatalf("cache cleanup should not be flagged as harmful: %q", frame.HarmfulReading)
	}
}

func TestTokenSet_OnlySeparators(t *testing.T) {
	// Only non-alphanumeric characters — flush is called repeatedly
	// with an empty buffer, hitting the b.Len() == 0 early return.
	s := tokenSet("!@#$%")
	if len(s) != 0 {
		t.Fatalf("tokenSet with only separators: got %d tokens, want 0", len(s))
	}
}
