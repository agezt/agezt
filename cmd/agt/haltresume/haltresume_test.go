// SPDX-License-Identifier: MIT

package haltresume

import (
	"strings"
	"testing"
)

// Integration tests (live daemon) live in cmd/agt — the package
// owns only the pure argument-parsing branches. The full happy
// path is covered by cmd/agt/main_test.go's dispatcher coverage.

func TestHalt_HelpExitsZero(t *testing.T) {
	var out, errb strings.Builder
	if code := Halt([]string{"--help"}, &out, &errb); code != 0 {
		t.Errorf("Halt(--help) returned %d, want 0", code)
	}
	if !strings.Contains(out.String(), "usage:") {
		t.Errorf("expected usage line, got: %q", out.String())
	}
}

func TestResume_HelpExitsZero(t *testing.T) {
	var out, errb strings.Builder
	if code := Resume([]string{"--help"}, &out, &errb); code != 0 {
		t.Errorf("Resume(--help) returned %d, want 0", code)
	}
	if !strings.Contains(out.String(), "usage:") {
		t.Errorf("expected usage line, got: %q", out.String())
	}
}

func TestHalt_UnexpectedArgExitsTwo(t *testing.T) {
	var out, errb strings.Builder
	if code := Halt([]string{"--bogus"}, &out, &errb); code != 2 {
		t.Errorf("Halt(--bogus) returned %d, want 2", code)
	}
}

func TestHalt_ReasonWithoutValueExitsTwo(t *testing.T) {
	var out, errb strings.Builder
	if code := Halt([]string{"--reason"}, &out, &errb); code != 2 {
		t.Errorf("Halt(--reason) returned %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "--reason needs a value") {
		t.Errorf("expected --reason-needs-value message, got: %q", errb.String())
	}
}

func TestReasonOrPlaceholder(t *testing.T) {
	if got := reasonOrPlaceholder(""); got != "—" {
		t.Errorf("empty reason: got %q, want %q", got, "—")
	}
	if got := reasonOrPlaceholder("ops drill"); got != "ops drill" {
		t.Errorf("non-empty reason: got %q, want %q", got, "ops drill")
	}
}
