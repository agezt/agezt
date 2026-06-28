// SPDX-License-Identifier: MIT

package channel

import (
	"strings"
	"testing"
)

func TestAllowlist(t *testing.T) {
	a := NewAllowlist([]string{"123", " 456 ", "", "  "})
	if !a.Allows("123") || !a.Allows("456") {
		t.Fatal("configured ids must be allowed (trimmed)")
	}
	if a.Allows("999") {
		t.Fatal("unconfigured id must be denied")
	}
	if a.Empty() {
		t.Fatal("allowlist with ids is not empty")
	}
}

func TestAllowlistEmptyFailsClosed(t *testing.T) {
	a := NewAllowlist(nil)
	if !a.Empty() {
		t.Fatal("no ids → Empty")
	}
	if a.Allows("anything") {
		t.Fatal("empty allowlist must deny everyone (fail-closed)")
	}
}

func TestParseAllowlist(t *testing.T) {
	a := parseAllowlist("11, 22 ,33")
	for _, id := range []string{"11", "22", "33"} {
		if !a.Allows(id) {
			t.Errorf("id %q should be allowed", id)
		}
	}
	if !parseAllowlist("").Empty() {
		t.Fatal("empty string → empty allowlist")
	}
}

func parseAllowlist(s string) Allowlist {
	return NewAllowlist(strings.Split(s, ","))
}
