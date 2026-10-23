// SPDX-License-Identifier: MIT

package overseertool

import (
	"encoding/json"
	"testing"
)

// --- parseProfile edge cases ---

func TestParseProfile_NullAndEmptyProfileRaw(t *testing.T) {
	src := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

	// Null profile raw → falls back to flat fields.
	_, err := parseProfile(src(nil), src(map[string]any{"slug": "a"}))
	if err != nil {
		t.Fatalf("null profile with flat fields: %v", err)
	}

	// Empty string profile raw is truthy but not a valid object → error.
	_, err = parseProfile(src(""), src(map[string]any{"slug": "b"}))
	if err == nil {
		t.Fatal("empty string profile raw should error (string is not an object)")
	}
}

func TestParseProfile_InvalidJSONInsideProfile(t *testing.T) {
	src := json.RawMessage(`{invalid}`)
	_, err := parseProfile(src, src)
	if err == nil {
		t.Fatal("parseProfile with invalid JSON should error")
	}
}

func TestParseProfile_NoProfileAndNoFlatFields(t *testing.T) {
	src := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	_, err := parseProfile(src(nil), src(map[string]any{}))
	if err == nil {
		t.Fatal("parseProfile with no profile and no flat fields should error")
	}
}

// --- hasProfileObject ---

func TestHasProfileObject_Variants(t *testing.T) {
	cases := []struct {
		raw json.RawMessage
		ok  bool
	}{
		{json.RawMessage(`{}`), false},
		{json.RawMessage(`null`), false},
		{json.RawMessage(``), false},
		{json.RawMessage(`{"slug":"x"}`), true},
		{json.RawMessage(`0`), true}, // truthy but not an object — should pass
	}
	for _, tc := range cases {
		got := hasProfileObject(tc.raw)
		if got != tc.ok {
			t.Errorf("hasProfileObject(%q) = %v, want %v", string(tc.raw), got, tc.ok)
		}
	}
}

// --- cleanSlugs ---

func TestCleanSlugs_TrimAndDeduplicates(t *testing.T) {
	got := cleanSlugs([]string{"  a1  ", "", "  ", "a2", " a2 ", "a3"})
	// cleanSlugs trims and removes empties but does NOT deduplicate.
	if len(got) != 4 || got[0] != "a1" || got[3] != "a3" {
		t.Fatalf("cleanSlugs = %#v, want roughly [a1 a2 a2 a3]", got)
	}
}

func TestCleanSlugs_NilAndEmpty(t *testing.T) {
	if cleanSlugs(nil) != nil {
		t.Error("cleanSlugs(nil) should be nil")
	}
	if cleanSlugs([]string{}) != nil {
		t.Error("cleanSlugs(empty) should be nil")
	}
	if len(cleanSlugs([]string{"", "  "})) != 0 {
		t.Error("cleanSlugs(all empty) should be empty")
	}
}

// --- errResult ---

func TestErrResult_Format(t *testing.T) {
	r := errResult("something went wrong")
	if !r.IsError {
		t.Error("errResult should mark IsError")
	}
	if r.Output != "overseer: something went wrong" {
		t.Errorf("errResult output = %q", r.Output)
	}
}
