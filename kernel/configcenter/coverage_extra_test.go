// SPDX-License-Identifier: MIT

package configcenter

import (
	"testing"
	"time"
)

func TestParseRating(t *testing.T) {
	for in, want := range map[string]Rating{
		"public": RatingPublic, "PUBLIC": RatingPublic,
		"internal": RatingInternal, "restricted": RatingRestricted, "secret": RatingSecret,
	} {
		got, err := ParseRating(in)
		if err != nil || got != want {
			t.Errorf("ParseRating(%q) = %q, %v, want %q", in, got, err, want)
		}
	}
	if _, err := ParseRating("bogus"); err == nil {
		t.Error("ParseRating('bogus') should error")
	}
}

func TestMaskValue(t *testing.T) {
	ap := &AccessPolicy{}
	if got := ap.maskValue("ab"); got != "**" {
		t.Errorf("maskValue('ab') = %q, want '**'", got)
	}
	if got := ap.maskValue("abcdefgh"); got != "****efgh" {
		t.Errorf("maskValue('abcdefgh') = %q, want '****efgh'", got)
	}
}

func TestHitlConfigTimeout(t *testing.T) {
	h := &HitlConfig{TimeoutMinutes: 5}
	if d := h.Timeout(); d != 5*time.Minute {
		t.Errorf("Timeout = %v, want 5m", d)
	}
}

func TestConfigEntryChainMethods(t *testing.T) {
	e := NewConfigEntry("k", "v")
	e.SetRating(RatingSecret).SetTags("a", "b").SetDescription("desc").SetAccessPolicy(PolicyAuto)
	if e.Rating != RatingSecret || e.Description != "desc" || e.AccessPolicy != PolicyAuto {
		t.Error("chain methods failed")
	}
}

func TestAllowDenyAgent(t *testing.T) {
	e := NewConfigEntry("k", "v")
	e.AllowAgent("agent1")
	if len(e.AllowedAgents) != 1 || e.AllowedAgents[0] != "agent1" {
		t.Error("AllowAgent failed")
	}
	e.DenyAgent("agent2")
	if len(e.ExcludedAgents) != 1 || e.ExcludedAgents[0] != "agent2" {
		t.Error("DenyAgent failed")
	}
}

func TestConfigErrorUnwrap(t *testing.T) {
	err := NewConfigError("E001", "test error")
	if err.Error() == "" || err.Unwrap() != nil {
		t.Error("NewConfigError/Unwrap failed")
	}
	ext := err.WithExtra("k", "v")
	if ext.Extra["k"] != "v" {
		t.Error("WithExtra failed to set extra")
	}
}

func TestClassifierOverrideLifecycle(t *testing.T) {
	sc := NewSecretClassifier()
	_, ok := sc.GetOverride("k")
	if ok {
		t.Error("GetOverride on fresh classifier should be false")
	}
	sc.SetOverride("k", RatingSecret)
	r, ok := sc.GetOverride("k")
	if !ok || r != RatingSecret {
		t.Error("GetOverride after Set failed")
	}
	sc.RemoveOverride("k")
	_, ok = sc.GetOverride("k")
	if ok {
		t.Error("GetOverride after Remove should be false")
	}
}

func TestSuggestRating(t *testing.T) {
	r, reason := NewSecretClassifier().SuggestRating("KEY", "secret-value")
	if r == "" || reason == "" {
		t.Error("SuggestRating should return rating and reason")
	}
}

func TestSplitLines(t *testing.T) {
	if got := splitLines(""); len(got) != 0 {
		t.Error("splitLines('') should be empty")
	}
	if got := splitLines("a\nb\nc\n"); len(got) != 3 {
		t.Errorf("splitLines multi-line = %d, want 3", len(got))
	}
	if got := splitLines("solo"); len(got) != 1 || got[0] != "solo" {
		t.Error("splitLines single failed")
	}
}
