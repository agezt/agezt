// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

func textEvent(kind event.Kind, text string) *event.Event {
	payload, _ := json.Marshal(map[string]any{"text": text})
	return &event.Event{Kind: kind, Subject: "agent.x.llm", Actor: "agent-x", Payload: payload}
}

// TestRenderEventHuman_TextFlag (M333): --text appends a one-line excerpt of the
// event's text payload (reasoning + answer tokens); without it the structured
// one-line format is byte-for-byte unchanged.
func TestRenderEventHuman_TextFlag(t *testing.T) {
	ev := textEvent(event.KindLLMReasoning, "Let me think about 6*7.")

	var off bytes.Buffer
	renderEventHuman(&off, ev, false)
	if strings.Contains(off.String(), "▸") || strings.Contains(off.String(), "Let me think") {
		t.Errorf("default (no --text) must not show payload text: %q", off.String())
	}
	if !strings.Contains(off.String(), string(event.KindLLMReasoning)) {
		t.Errorf("structured line should still show the kind: %q", off.String())
	}

	var on bytes.Buffer
	renderEventHuman(&on, ev, true)
	if !strings.Contains(on.String(), "▸ Let me think about 6*7.") {
		t.Errorf("--text should append the reasoning excerpt: %q", on.String())
	}
}

// TestEventTextExcerpt covers extraction, whitespace collapsing, truncation, and
// the empty cases (no payload / no text field).
func TestEventTextExcerpt(t *testing.T) {
	cases := []struct {
		name string
		ev   *event.Event
		want string
	}{
		{"plain", textEvent(event.KindLLMToken, "hello world"), "hello world"},
		{"collapse-newlines", textEvent(event.KindLLMReasoning, "step 1\n\nstep 2\tdone"), "step 1 step 2 done"},
		{"no-text-field", &event.Event{Kind: event.KindLLMResponse, Payload: json.RawMessage(`{"reasoning_chars":42}`)}, ""},
		{"no-payload", &event.Event{Kind: event.KindTaskCompleted}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := eventTextExcerpt(c.ev); got != c.want {
				t.Errorf("excerpt=%q want %q", got, c.want)
			}
		})
	}

	// Long text is truncated with an ellipsis and stays bounded.
	long := strings.Repeat("a", 500)
	got := eventTextExcerpt(textEvent(event.KindLLMToken, long))
	if !strings.HasSuffix(got, "…") {
		t.Errorf("long text should be ellipsised: %q", got)
	}
	if len([]rune(got)) > 170 { // 160 chars + ellipsis, generous bound
		t.Errorf("excerpt too long: %d runes", len([]rune(got)))
	}
}
