// SPDX-License-Identifier: MIT

package channel

import (
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

// TestConversationHistory_SkipsMalformedPayload covers the json.Unmarshal error
// branch: a malformed event payload is skipped rather than aborting the fold.
func TestConversationHistory_SkipsMalformedPayload(t *testing.T) {
	bad := &event.Event{Kind: event.KindChannelInbound, Payload: []byte("{not json")}
	r := &fakeRanger{events: []*event.Event{
		bad,
		ev(event.KindChannelInbound, "telegram", "9", "hi"),
		ev(event.KindChannelOutbound, "telegram", "9", "hello"),
	}}
	got := ConversationHistory(r, "telegram", "9", "", "", 10)
	if got == "" {
		t.Fatal("expected a transcript ignoring the malformed event")
	}
}

// TestInstanceKey_EmptyLabel covers the default-account (empty label) branch.
func TestInstanceKey_EmptyLabel(t *testing.T) {
	if got := InstanceKey("telegram", ""); got != "telegram" {
		t.Fatalf("InstanceKey(kind, \"\") = %q, want \"telegram\"", got)
	}
	if got := InstanceKey("telegram", "work"); got != "telegram#work" {
		t.Fatalf("InstanceKey labelled = %q, want \"telegram#work\"", got)
	}
}

// TestRuneUnits_InvalidRune covers the fallback branch where utf16.RuneLen
// returns <= 0 for an invalid rune (e.g. an unpaired surrogate), so runeUnits
// counts it as one unit (the replacement char).
func TestRuneUnits_InvalidRune(t *testing.T) {
	if u := runeUnits(rune(0xD800)); u != 1 { // surrogate: RuneLen == -1
		t.Fatalf("runeUnits(0xD800) = %d, want 1", u)
	}
	if u := runeUnits(rune(0x110000)); u != 1 { // out of range: RuneLen == -1
		t.Fatalf("runeUnits(0x110000) = %d, want 1", u)
	}
	// Sanity: a normal BMP rune is one unit, an astral rune is two.
	if u := runeUnits('a'); u != 1 {
		t.Fatalf("runeUnits('a') = %d, want 1", u)
	}
	if u := runeUnits('𝄞'); u != 2 { // U+1D11E needs a surrogate pair
		t.Fatalf("runeUnits(astral) = %d, want 2", u)
	}
}
