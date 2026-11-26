// SPDX-License-Identifier: MIT

package webui

import (
	"net/http"
	"strings"
	"testing"
)

// The console's own answers are the CONTRACT's rows, byte for byte.
//
// The rows moved out of the console's hand-written maps into the console's
// own contract (kernel/contract/consoleapi). These tests pin the BYTES: a
// struct that changed its member order, or a field that appeared out of
// nowhere, would move the wire — and the wire is what the browser reads.

func TestTheConsoleServesTheContractAuthMetaBytes(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	rec := httpJSON(t, s.Handler(), http.MethodGet, "/api/authmeta", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	want := `{"authed":false,"password_required":false}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("the auth meta answer's bytes moved:\n got %s\nwant %s", got, want)
	}
}

func TestTheConsoleServesTheContractLoginBytes(t *testing.T) {
	// No gate configured: the answer says so.
	s, _ := newServer(t, &fakeCaller{}, "secret")
	rec := httpJSON(t, s.Handler(), http.MethodPost, "/api/login", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	want := `{"ok":true,"password_required":false}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("the login answer's bytes moved:\n got %s\nwant %s", got, want)
	}

	// A gate configured and passed: the answer says only that it worked.
	gated, _ := newServer(t, &fakeCaller{}, "secret")
	gated.SetPasswordFn(func() string { return "s3cret" })
	rec = httpJSON(t, gated.Handler(), http.MethodPost, "/api/login", `{"password":"s3cret"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	want = `{"ok":true}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("the gated login answer's bytes moved:\n got %s\nwant %s", got, want)
	}
}

func TestTheConsoleServesTheContractLogoutBytes(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	rec := httpJSON(t, s.Handler(), http.MethodPost, "/api/logout", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	want := `{"ok":true}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("the logout answer's bytes moved:\n got %s\nwant %s", got, want)
	}
}

func TestTheConsoleServesTheContractVoiceStatusBytes(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	rec := httpJSON(t, s.Handler(), http.MethodGet, "/api/voice/status?token=secret", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	want := `{"stt":{"configured":false},"tts":{"configured":false}}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("the voice status answer's bytes moved:\n got %s\nwant %s", got, want)
	}
}

func TestTheConsoleServesTheContractWorkflowHookBytes(t *testing.T) {
	// The accepted answer: the run was taken and will finish in the background.
	accepted := &fakeCaller{result: map[string]any{
		"accepted": true, "correlation_id": "run-1", "workflow": "hooked",
	}}
	s, _ := newServer(t, accepted, "secret")
	rec := httpJSON(t, s.Handler(), http.MethodPost, "/hooks/hooked", `{"x":"dis"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	want := `{"accepted":true,"correlation_id":"run-1","workflow":"hooked"}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("the workflow answer's bytes moved:\n got %s\nwant %s", got, want)
	}

	// The reply answer: the run finished while the caller waited.
	reply := &fakeCaller{result: map[string]any{
		"workflow": "echoer", "correlation_id": "run-2",
		"executed": []any{"start", "shape"}, "outputs": map[string]any{"shape": "pong 42"},
	}}
	sReply, _ := newServer(t, reply, "secret")
	rec = httpJSON(t, sReply.Handler(), http.MethodPost, "/hooks/echoer", `{"ping":"42"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	want = `{"correlation_id":"run-2","executed":["start","shape"],"ok":true,` +
		`"outputs":{"shape":"pong 42"},"workflow":"echoer"}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("the workflow reply answer's bytes moved:\n got %s\nwant %s", got, want)
	}
}
