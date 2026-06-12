// SPDX-License-Identifier: MIT

package restapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/agezt/agezt/kernel/board"
)

// mailboxServer is newServer plus a wired board store, mirroring the daemon's
// SetMailbox call. Returns the server and the collected notifications.
func mailboxServer(t *testing.T, token string) (*Server, *[]board.Message) {
	t.Helper()
	s := newServer(t, &fakeEngine{model: "m"}, token)
	st, err := board.Open(t.TempDir())
	if err != nil {
		t.Fatalf("board.Open: %v", err)
	}
	var notified []board.Message
	s.SetMailbox(st, func(m board.Message, _ string) { notified = append(notified, m) })
	return s, &notified
}

func decode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return v
}

// TestMailbox_SendInboxReplyAck walks the full app-facing arc: DM an agent,
// see it in the inbox, reply back, broadcast + ack, and the notifier fires for
// every write.
func TestMailbox_SendInboxReplyAck(t *testing.T) {
	s, notified := mailboxServer(t, "tok")

	// DM: to + text, topic defaults to "dm".
	rec := do(t, s, http.MethodPost, "/api/v1/mailbox/messages",
		`{"from":"myapp","to":"researcher","text":"deploy target?"}`, "tok")
	if rec.Code != http.StatusCreated {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}
	msg := decode(t, rec.Body.Bytes())["message"].(map[string]any)
	id, _ := msg["id"].(string)
	if id == "" || msg["topic"] != "dm" || msg["to"] != "researcher" {
		t.Fatalf("message view wrong: %+v", msg)
	}

	// Inbox shows it.
	rec = do(t, s, http.MethodGet, "/api/v1/mailbox/inbox?name=researcher", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("inbox: %d", rec.Code)
	}
	if got := decode(t, rec.Body.Bytes())["count"].(float64); got != 1 {
		t.Fatalf("inbox count = %v, want 1", got)
	}

	// Reply threads back to the sender automatically.
	rec = do(t, s, http.MethodPost, "/api/v1/mailbox/messages",
		`{"from":"researcher","reply_to":"`+id+`","text":"prod-eu"}`, "tok")
	if rec.Code != http.StatusCreated {
		t.Fatalf("reply: %d %s", rec.Code, rec.Body.String())
	}
	rep := decode(t, rec.Body.Bytes())["message"].(map[string]any)
	if rep["to"] != "myapp" || rep["reply_to"] != id {
		t.Fatalf("reply view wrong: %+v", rep)
	}
	rec = do(t, s, http.MethodGet, "/api/v1/mailbox/messages/"+id+"/replies", "", "tok")
	if got := decode(t, rec.Body.Bytes())["count"].(float64); got != 1 {
		t.Fatalf("replies count = %v, want 1", got)
	}

	// Broadcast, then ack clears it for the acker only.
	rec = do(t, s, http.MethodPost, "/api/v1/mailbox/messages",
		`{"from":"myapp","to":"*","text":"heads-up"}`, "tok")
	bcID := decode(t, rec.Body.Bytes())["message"].(map[string]any)["id"].(string)
	rec = do(t, s, http.MethodPost, "/api/v1/mailbox/messages/"+bcID+"/ack",
		`{"by":"researcher"}`, "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("ack: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, s, http.MethodGet, "/api/v1/mailbox/inbox?name=researcher", "", "tok")
	if got := decode(t, rec.Body.Bytes())["count"].(float64); got != 0 {
		t.Fatalf("acked+answered inbox should be empty: %v", got)
	}
	rec = do(t, s, http.MethodGet, "/api/v1/mailbox/inbox?name=writer", "", "tok")
	if got := decode(t, rec.Body.Bytes())["count"].(float64); got != 1 {
		t.Fatalf("broadcast must still wait for writer: %v", got)
	}

	// Topic post + read + topics.
	rec = do(t, s, http.MethodPost, "/api/v1/mailbox/messages",
		`{"from":"myapp","topic":"status","text":"shipped"}`, "tok")
	if rec.Code != http.StatusCreated {
		t.Fatalf("post: %d", rec.Code)
	}
	rec = do(t, s, http.MethodGet, "/api/v1/mailbox/messages?topic=status", "", "tok")
	if got := decode(t, rec.Body.Bytes())["count"].(float64); got != 1 {
		t.Fatalf("read count = %v, want 1", got)
	}
	rec = do(t, s, http.MethodGet, "/api/v1/mailbox/topics", "", "tok")
	topics := decode(t, rec.Body.Bytes())["topics"].(map[string]any)
	if topics["status"].(float64) != 1 {
		t.Fatalf("topics wrong: %+v", topics)
	}

	// DM, reply, broadcast, topic post — four writes, four notifications.
	if len(*notified) != 4 {
		t.Fatalf("notifier fired %d times, want 4", len(*notified))
	}
}

// TestMailbox_ValidationAndAuth covers the unhappy paths: auth required, 503
// when unwired, missing fields, unknown ids, wrong methods.
func TestMailbox_ValidationAndAuth(t *testing.T) {
	s, _ := mailboxServer(t, "tok")

	// Token required on every mailbox route.
	for _, path := range []string{"/api/v1/mailbox/messages", "/api/v1/mailbox/inbox", "/api/v1/mailbox/topics"} {
		if rec := do(t, s, http.MethodGet, path, "", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s without token: %d, want 401", path, rec.Code)
		}
	}

	// Unwired mailbox answers 503.
	bare := newServer(t, &fakeEngine{model: "m"}, "tok")
	if rec := do(t, bare, http.MethodGet, "/api/v1/mailbox/messages", "", "tok"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unwired: %d, want 503", rec.Code)
	}

	// Missing text / missing topic+to / unknown reply target.
	if rec := do(t, s, http.MethodPost, "/api/v1/mailbox/messages", `{"topic":"x"}`, "tok"); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing text: %d", rec.Code)
	}
	if rec := do(t, s, http.MethodPost, "/api/v1/mailbox/messages", `{"text":"x"}`, "tok"); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing topic and to: %d", rec.Code)
	}
	if rec := do(t, s, http.MethodPost, "/api/v1/mailbox/messages", `{"text":"x","reply_to":"nope"}`, "tok"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown reply target: %d", rec.Code)
	}

	// Inbox needs a name; ack needs by and a real id; unknown sub-action 404s.
	if rec := do(t, s, http.MethodGet, "/api/v1/mailbox/inbox", "", "tok"); rec.Code != http.StatusBadRequest {
		t.Fatalf("inbox without name: %d", rec.Code)
	}
	if rec := do(t, s, http.MethodPost, "/api/v1/mailbox/messages/some-id/ack", `{}`, "tok"); rec.Code != http.StatusBadRequest {
		t.Fatalf("ack without by: %d", rec.Code)
	}
	if rec := do(t, s, http.MethodPost, "/api/v1/mailbox/messages/nope/ack", `{"by":"a"}`, "tok"); rec.Code != http.StatusNotFound {
		t.Fatalf("ack unknown id: %d", rec.Code)
	}
	if rec := do(t, s, http.MethodGet, "/api/v1/mailbox/messages/nope/wat", "", "tok"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown action: %d", rec.Code)
	}

	// Method discipline.
	if rec := do(t, s, http.MethodDelete, "/api/v1/mailbox/messages", "", "tok"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE messages: %d", rec.Code)
	}
	if rec := do(t, s, http.MethodPost, "/api/v1/mailbox/topics", "", "tok"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST topics: %d", rec.Code)
	}
}
