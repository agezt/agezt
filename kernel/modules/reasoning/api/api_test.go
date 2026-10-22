// SPDX-License-Identifier: MIT

package api

import (
	"reflect"
	"testing"
)

// TestNormalizeMembers: the panel rule the council and the conductor share —
// trim, drop the modelless seats, name the blank ones by position, dedupe.
func TestNormalizeMembers(t *testing.T) {
	got := NormalizeMembers([]CouncilMember{
		{Seat: "  Alpha ", Model: " model-a "},
		{Seat: "", Model: "model-b"},
		{Seat: "NoModel", Model: "  "},
		{Seat: "Alpha", Model: "model-a"},
	})
	want := []CouncilMember{
		{Seat: "Alpha", Model: "model-a"},
		{Seat: "Elder 2", Model: "model-b"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("members = %+v, want %+v", got, want)
	}
}

// TestNormalizeMembersEmpty: an empty panel is an empty list, never nil — the
// council reports "no members available" off its length, not off a nil check.
func TestNormalizeMembersEmpty(t *testing.T) {
	got := NormalizeMembers(nil)
	if got == nil {
		t.Fatal("an empty panel must be an empty list, not nil")
	}
	if len(got) != 0 {
		t.Fatalf("members = %+v", got)
	}
}

// TestOrPlaceholder: an empty answer still says something.
func TestOrPlaceholder(t *testing.T) {
	if got := OrPlaceholder("   "); got != "(no answer)" {
		t.Errorf("OrPlaceholder(blank) = %q", got)
	}
	if got := OrPlaceholder(" real "); got != " real " {
		t.Errorf("OrPlaceholder must not touch a real answer: %q", got)
	}
}

// TestClip: the journal payload bound is rune-safe, so a multi-byte answer is
// clipped by characters and never split mid-rune.
func TestClip(t *testing.T) {
	if got := Clip("short", 10); got != "short" {
		t.Errorf("Clip(short) = %q", got)
	}
	got := Clip("ğğğğğ", 3)
	if len([]rune(got)) != 4 {
		t.Errorf("Clip must keep max runes plus the ellipsis: %q", got)
	}
	if got[len(got)-3:] != "…" {
		t.Errorf("Clip must end with the ellipsis: %q", got)
	}
}
