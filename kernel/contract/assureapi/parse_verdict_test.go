// SPDX-License-Identifier: MIT

package assureapi

import "testing"

// TestParseVerdict_MalformedJSON: a half-written or broken object is not a
// verdict. The kernel treats an unreadable reply as "not complete", so the
// parser must refuse rather than hand back a zero verdict that looks like a
// judgement.
func TestParseVerdict_MalformedJSON(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"truncated object", `{"complete": true,`},
		{"bad value", `{bad}`},
		{"partial json", `{"complete"`},
		{"prose with bad json", `verdict: {bad json} here`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := ParseVerdict(tt.in)
			if ok {
				t.Errorf("ParseVerdict(%q) = (%+v, true), want (_, false)", tt.in, v)
			}
		})
	}
}

// TestParseVerdict_FencedOnly: a reply that is ONLY a fenced block (no
// surrounding prose) still carries a verdict.
func TestParseVerdict_FencedOnly(t *testing.T) {
	v, ok := ParseVerdict("```\n{\"complete\": true}\n```")
	if !ok {
		t.Fatal("ParseVerdict should parse fenced JSON")
	}
	if !v.Complete {
		t.Error("expected complete=true")
	}
}

// TestParseVerdict_OnlyFenceNoJSON: a fence with no JSON inside is a fence,
// not a verdict.
func TestParseVerdict_OnlyFenceNoJSON(t *testing.T) {
	v, ok := ParseVerdict("```")
	if ok {
		t.Errorf("ParseVerdict with only fence should fail, got %+v", v)
	}
}
