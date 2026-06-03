// SPDX-License-Identifier: MIT

package peer

import (
	"strings"
	"testing"
)

// TestRemoteRun_ForwardsModel proves a pinned model is forwarded to the peer's
// /api/v1/runs body so the peer routes the delegated task to that model.
func TestRemoteRun_ForwardsModel(t *testing.T) {
	var body string
	tool := &Tool{
		Peers: map[string]Peer{"nodeB": {Name: "nodeB", URL: "http://host:8800"}},
		post:  fakePost(200, `{"correlation_id":"c","status":"completed","answer":"ok","model":"opus"}`, nil, nil, &body),
	}
	out, isErr := invoke(t, tool, map[string]string{"peer": "nodeB", "task": "do it", "model": "opus"})
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}
	if !strings.Contains(body, `"model":"opus"`) {
		t.Errorf("model not forwarded in body: %s", body)
	}
	if !strings.Contains(body, `"intent":"do it"`) {
		t.Errorf("intent missing: %s", body)
	}
	// The peer echoes the routed model; the footer records it for the transcript.
	if !strings.Contains(out, "model=opus") {
		t.Errorf("footer missing model: %s", out)
	}
}

// TestRemoteRun_OmitsModelByDefault proves the default path is unchanged: with no
// model pinned, the body carries only the intent (no "model" key), so the peer
// uses its own default model exactly as before.
func TestRemoteRun_OmitsModelByDefault(t *testing.T) {
	var body string
	tool := &Tool{
		Peers: map[string]Peer{"a": {Name: "a", URL: "http://h:1"}},
		post:  fakePost(200, `{"status":"completed","answer":"ok","correlation_id":"c"}`, nil, nil, &body),
	}
	out, isErr := invoke(t, tool, map[string]string{"task": "hello"})
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}
	if strings.Contains(body, "model") {
		t.Errorf("body must not carry a model key when none is pinned: %s", body)
	}
	// Footer has no model segment when the peer reports none.
	if strings.Contains(out, "model=") {
		t.Errorf("footer must not show a model when none was used: %s", out)
	}
}

// TestRemoteRun_BlankModelTreatedAsUnset proves a whitespace-only model is trimmed
// to empty and not forwarded (same as omitting it).
func TestRemoteRun_BlankModelTreatedAsUnset(t *testing.T) {
	var body string
	tool := &Tool{
		Peers: map[string]Peer{"a": {Name: "a", URL: "http://h:1"}},
		post:  fakePost(200, `{"status":"completed","answer":"ok","correlation_id":"c"}`, nil, nil, &body),
	}
	if _, isErr := invoke(t, tool, map[string]string{"task": "hi", "model": "   "}); isErr {
		t.Fatal("unexpected error")
	}
	if strings.Contains(body, "model") {
		t.Errorf("blank model must not be forwarded: %s", body)
	}
}
