// SPDX-License-Identifier: MIT

package restapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

// --- tokenText ---

func TestTokenText_NilEvent(t *testing.T) {
	if tokenText(nil) != "" {
		t.Error("tokenText(nil) should return empty")
	}
}

func TestTokenText_WrongKind(t *testing.T) {
	ev := &event.Event{Kind: event.KindLLMRequest, Payload: json.RawMessage(`{"text":"x"}`)}
	if tokenText(ev) != "" {
		t.Error("tokenText with wrong kind should return empty")
	}
}

func TestTokenText_EmptyPayload(t *testing.T) {
	ev := &event.Event{Kind: event.KindLLMToken}
	if tokenText(ev) != "" {
		t.Error("tokenText with empty payload should return empty")
	}
}

func TestTokenText_UnmarshalError(t *testing.T) {
	ev := &event.Event{Kind: event.KindLLMToken, Payload: json.RawMessage(`not json`)}
	if tokenText(ev) != "" {
		t.Error("tokenText with invalid payload should return empty")
	}
}

func TestTokenText_Success(t *testing.T) {
	ev := &event.Event{Kind: event.KindLLMToken, Payload: json.RawMessage(`{"text":"hello"}`)}
	if txt := tokenText(ev); txt != "hello" {
		t.Errorf("tokenText = %q, want hello", txt)
	}
}

// --- streamClientKey ---

func TestStreamClientKey_WithPort(t *testing.T) {
	r := &http.Request{RemoteAddr: "192.168.1.1:45678"}
	if key := streamClientKey(r); key != "192.168.1.1" {
		t.Errorf("streamClientKey = %q, want 192.168.1.1", key)
	}
}

func TestStreamClientKey_NoPort(t *testing.T) {
	r := &http.Request{RemoteAddr: "unix-socket-path"}
	if key := streamClientKey(r); key != "unix-socket-path" {
		t.Errorf("streamClientKey(no-port) = %q, want raw addr", key)
	}
}
