// SPDX-License-Identifier: MIT

package assure

import "testing"

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

func TestParseVerdict_FencedOnly(t *testing.T) {
	// Input that is ONLY a fenced block (no surrounding prose)
	v, ok := ParseVerdict("```\n{\"complete\": true}\n```")
	if !ok {
		t.Fatal("ParseVerdict should parse fenced JSON")
	}
	if !v.Complete {
		t.Error("expected complete=true")
	}
}

func TestParseVerdict_OnlyFenceNoJSON(t *testing.T) {
	v, ok := ParseVerdict("```")
	if ok {
		t.Errorf("ParseVerdict with only fence should fail, got %+v", v)
	}
}
