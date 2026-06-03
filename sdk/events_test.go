// SPDX-License-Identifier: MIT

package sdk

import (
	"encoding/json"
	"testing"
)

func TestTokenText(t *testing.T) {
	if txt, ok := TokenText(&Event{Kind: "llm.token", Payload: json.RawMessage(`{"text":"hello"}`)}); !ok || txt != "hello" {
		t.Errorf("token text = (%q,%v), want (hello,true)", txt, ok)
	}
	// Wrong kind, empty delta, and nil all yield ok=false.
	if _, ok := TokenText(&Event{Kind: "tool.invoked", Payload: json.RawMessage(`{"tool":"file"}`)}); ok {
		t.Error("non-token event should yield false")
	}
	if _, ok := TokenText(&Event{Kind: "llm.token", Payload: json.RawMessage(`{"text":""}`)}); ok {
		t.Error("empty delta should yield false")
	}
	if _, ok := TokenText(nil); ok {
		t.Error("nil event should yield false")
	}
}

func TestToolCall(t *testing.T) {
	if name, ok := ToolCall(&Event{Kind: "tool.invoked", Payload: json.RawMessage(`{"tool":"http","input":{}}`)}); !ok || name != "http" {
		t.Errorf("tool call = (%q,%v), want (http,true)", name, ok)
	}
	if _, ok := ToolCall(&Event{Kind: "llm.token", Payload: json.RawMessage(`{"text":"x"}`)}); ok {
		t.Error("non-tool event should yield false")
	}
	if _, ok := ToolCall(nil); ok {
		t.Error("nil event should yield false")
	}
}

func TestIsTerminal(t *testing.T) {
	if !IsTerminal(&Event{Kind: "task.completed"}) {
		t.Error("task.completed should be terminal")
	}
	if !IsTerminal(&Event{Kind: "task.failed"}) {
		t.Error("task.failed should be terminal")
	}
	if IsTerminal(&Event{Kind: "llm.token"}) {
		t.Error("llm.token should not be terminal")
	}
	if IsTerminal(nil) {
		t.Error("nil should not be terminal")
	}
}
