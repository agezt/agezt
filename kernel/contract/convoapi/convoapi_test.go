// SPDX-License-Identifier: MIT

package convoapi

import "testing"

func TestTranscriptIntent_SingleUserVerbatim(t *testing.T) {
	got := TranscriptIntent([]Turn{{Role: "user", Text: "  turn off the light  "}})
	if got != "turn off the light" {
		t.Errorf("single user turn = %q, want clean verbatim", got)
	}
}

func TestTranscriptIntent_MultiTurnTranscript(t *testing.T) {
	got := TranscriptIntent([]Turn{
		{Role: "user", Text: "remember the number 7"},
		{Role: "assistant", Text: "Got it, 7."},
		{Role: "user", Text: "what was it?"},
	})
	want := "User: remember the number 7\nAssistant: Got it, 7.\nUser: what was it?"
	if got != want {
		t.Errorf("transcript =\n%q\nwant\n%q", got, want)
	}
}

func TestTranscriptIntent_SystemHoisted(t *testing.T) {
	got := TranscriptIntent([]Turn{
		{Role: "system", Text: "be terse"},
		{Role: "user", Text: "hi"},
		{Role: "assistant", Text: "hello"},
		{Role: "user", Text: "bye"},
	})
	want := "be terse\n\nUser: hi\nAssistant: hello\nUser: bye"
	if got != want {
		t.Errorf("system-hoisted transcript =\n%q\nwant\n%q", got, want)
	}
}

func TestTranscriptIntent_SkipsEmptyTurns(t *testing.T) {
	got := TranscriptIntent([]Turn{
		{Role: "user", Text: "only this"},
		{Role: "assistant", Text: "   "},
	})
	// The blank assistant turn is dropped, leaving a single user turn → verbatim.
	if got != "only this" {
		t.Errorf("got %q, want %q", got, "only this")
	}
}

func TestTranscriptIntent_UnknownRoleVerbatimLine(t *testing.T) {
	got := TranscriptIntent([]Turn{
		{Role: "user", Text: "a"},
		{Role: "tool", Text: "tool output"},
		{Role: "user", Text: "b"},
	})
	want := "User: a\ntool output\nUser: b"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTranscriptIntent_Empty(t *testing.T) {
	if got := TranscriptIntent(nil); got != "" {
		t.Errorf("nil turns = %q, want empty", got)
	}
}

// TestTranscriptIntent_HoistsTheDeveloperRole: "developer" is leading
// guidance, exactly like "system" — it is hoisted to the front, not
// rendered as a line of the dialogue.
func TestTranscriptIntent_HoistsTheDeveloperRole(t *testing.T) {
	got := TranscriptIntent([]Turn{
		{Role: "developer", Text: "be terse"},
		{Role: "user", Text: "hi"},
		{Role: "assistant", Text: "hello"},
	})
	want := "be terse\n\nUser: hi\nAssistant: hello"
	if got != want {
		t.Errorf("developer-hoisted transcript =\n%q\nwant\n%q", got, want)
	}
}

// TestTranscriptIntent_OneUserAndOneAssistantIsATranscript: the verbatim
// shortcut is for a LONE user turn — a conversation that also has an
// assistant turn is a transcript, even when only one of the turns is the
// user's.
func TestTranscriptIntent_OneUserAndOneAssistantIsATranscript(t *testing.T) {
	got := TranscriptIntent([]Turn{
		{Role: "user", Text: "what is the weather"},
		{Role: "assistant", Text: "it is fine"},
	})
	want := "User: what is the weather\nAssistant: it is fine"
	if got != want {
		t.Errorf("transcript =\n%q\nwant\n%q", got, want)
	}
}

// TestTranscriptIntent_TheRoleIsCaseInsensitive: the role vocabulary is
// matched case-insensitively, because the surfaces that carry a history
// (the OpenAI-compatible API, the Web UI) spell roles however their client
// spells them.
func TestTranscriptIntent_TheRoleIsCaseInsensitive(t *testing.T) {
	got := TranscriptIntent([]Turn{
		{Role: "User", Text: "remember the number 7"},
		{Role: "ASSISTANT", Text: "Got it, 7."},
		{Role: "User", Text: "what was it?"},
	})
	want := "User: remember the number 7\nAssistant: Got it, 7.\nUser: what was it?"
	if got != want {
		t.Errorf("transcript =\n%q\nwant\n%q", got, want)
	}
}
