// SPDX-License-Identifier: MIT

package channel

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

// fakeRanger replays a fixed slice of events in order.
type fakeRanger struct{ events []*event.Event }

func (f *fakeRanger) Range(fn func(*event.Event) error) error {
	for _, e := range f.events {
		if err := fn(e); err != nil {
			return err
		}
	}
	return nil
}

func ev(kind event.Kind, ckind, cid, text string) *event.Event {
	p, _ := json.Marshal(map[string]any{"channel_kind": ckind, "channel_id": cid, "text": text})
	return &event.Event{Kind: kind, Payload: p}
}

func TestConversationHistory_BuildsTranscript(t *testing.T) {
	r := &fakeRanger{events: []*event.Event{
		ev(event.KindChannelInbound, "telegram", "42", "hi"),
		ev(event.KindChannelOutbound, "telegram", "42", "Hello! How can I help?"),
		ev(event.KindChannelInbound, "telegram", "42", "what's the capital of France?"),
	}}
	got := ConversationHistory(r, "telegram", "42", 10)
	if got == "" {
		t.Fatal("expected a transcript, got empty")
	}
	for _, want := range []string{"user: hi", "assistant: Hello! How can I help?", "user: what's the capital of France?"} {
		if !strings.Contains(got, want) {
			t.Errorf("transcript missing %q\n--- got ---\n%s", want, got)
		}
	}
	// Oldest first: "hi" precedes the capital question.
	if strings.Index(got, "hi") > strings.Index(got, "capital") {
		t.Error("transcript should be oldest-first")
	}
}

func TestConversationHistory_NoPriorContextIsEmpty(t *testing.T) {
	// Only the just-received message exists → no prior context → "".
	r := &fakeRanger{events: []*event.Event{
		ev(event.KindChannelInbound, "slack", "C1", "first message"),
	}}
	if got := ConversationHistory(r, "slack", "C1", 10); got != "" {
		t.Errorf("single message should yield no transcript, got %q", got)
	}
}

func TestConversationHistory_IsolatesConversation(t *testing.T) {
	// Messages from other channels/ids must not leak into this conversation.
	r := &fakeRanger{events: []*event.Event{
		ev(event.KindChannelInbound, "discord", "D9", "mine 1"),
		ev(event.KindChannelInbound, "discord", "OTHER", "not mine"),
		ev(event.KindChannelInbound, "slack", "D9", "wrong kind, same id"),
		ev(event.KindChannelOutbound, "discord", "D9", "mine 2"),
	}}
	got := ConversationHistory(r, "discord", "D9", 10)
	if strings.Contains(got, "not mine") || strings.Contains(got, "wrong kind") {
		t.Errorf("transcript leaked another conversation:\n%s", got)
	}
	if !strings.Contains(got, "mine 1") || !strings.Contains(got, "mine 2") {
		t.Errorf("transcript missing this conversation's messages:\n%s", got)
	}
}

func TestConversationHistory_RespectsLimit(t *testing.T) {
	var evs []*event.Event
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		evs = append(evs, ev(event.KindChannelInbound, "telegram", "1", s))
	}
	r := &fakeRanger{events: evs}
	got := ConversationHistory(r, "telegram", "1", 2)
	// Only the last two ("d","e") survive.
	if strings.Contains(got, "user: a") || strings.Contains(got, "user: c") {
		t.Errorf("limit not applied (should keep only last 2):\n%s", got)
	}
	if !strings.Contains(got, "user: d") || !strings.Contains(got, "user: e") {
		t.Errorf("limit dropped the wrong messages:\n%s", got)
	}
}

func TestConversationHistory_Disabled(t *testing.T) {
	r := &fakeRanger{events: []*event.Event{
		ev(event.KindChannelInbound, "telegram", "1", "a"),
		ev(event.KindChannelInbound, "telegram", "1", "b"),
	}}
	if got := ConversationHistory(r, "telegram", "1", 0); got != "" {
		t.Errorf("limit 0 should disable history, got %q", got)
	}
	if got := ConversationHistory(nil, "telegram", "1", 10); got != "" {
		t.Errorf("nil ranger should yield empty, got %q", got)
	}
}
