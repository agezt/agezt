// SPDX-License-Identifier: MIT

package imessage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

func TestParseWebhook(t *testing.T) {
	body := []byte(`{"type":"new-message","data":{"guid":"M1","text":"hi there","isFromMe":false,"handle":{"address":"+15551234567"},"chats":[{"guid":"iMessage;-;+15551234567"}]}}`)
	m, ok := parseWebhook(body)
	if !ok || m.text != "hi there" || m.sender != "+15551234567" || m.chatGUID != "iMessage;-;+15551234567" || m.id != "M1" {
		t.Fatalf("parseWebhook = %+v ok=%v", m, ok)
	}
	// our own echoes (isFromMe) are ignored.
	if _, ok := parseWebhook([]byte(`{"type":"new-message","data":{"text":"x","isFromMe":true}}`)); ok {
		t.Fatal("isFromMe should be ignored")
	}
	// non-message events are ignored.
	if _, ok := parseWebhook([]byte(`{"type":"typing-indicator","data":{}}`)); ok {
		t.Fatal("non new-message event should be ignored")
	}
}

func TestChatGUID(t *testing.T) {
	if got := chatGUID("+15551234567"); got != "iMessage;-;+15551234567" {
		t.Fatalf("bare address: %q", got)
	}
	if got := chatGUID("iMessage;-;a@b.com"); got != "iMessage;-;a@b.com" {
		t.Fatalf("full guid should pass through: %q", got)
	}
	if got := chatGUID("iMessage;+;chat123"); got != "iMessage;+;chat123" {
		t.Fatalf("group guid should pass through: %q", got)
	}
}

func TestSendShape(t *testing.T) {
	var path, query, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		query = r.URL.Query().Get("password")
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}))
	defer srv.Close()
	ch := New(Config{BaseURL: srv.URL, Password: "secret", HTTPClient: srv.Client()})
	if err := ch.Send(context.Background(), channel.Outbound{ChannelID: "+15551234567", Text: "yo"}); err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/message/text" || query != "secret" {
		t.Fatalf("path=%q password=%q", path, query)
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	if p["chatGuid"] != "iMessage;-;+15551234567" || p["message"] != "yo" || p["method"] != DefaultMethod {
		t.Fatalf("body = %s", body)
	}
	if g, _ := p["tempGuid"].(string); !strings.HasPrefix(g, "agezt-") {
		t.Fatalf("tempGuid = %v", p["tempGuid"])
	}
}

func TestInboundDispatchRepliesWhenAllowed(t *testing.T) {
	var sends int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sends++
	}))
	defer srv.Close()
	ch := New(Config{
		BaseURL:   srv.URL,
		Allowlist: channel.NewAllowlist([]string{"+15551234567"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{Text: "pong"}, nil
		},
		HTTPClient: srv.Client(),
	})
	ch.dispatch(context.Background(), inbound{chatGUID: "iMessage;-;+15551234567", sender: "+15551234567", text: "ping", id: "X1"})
	if sends != 1 {
		t.Fatalf("expected 1 reply send, got %d", sends)
	}
	// dedup: same id does not re-dispatch.
	ch.dispatch(context.Background(), inbound{chatGUID: "iMessage;-;+15551234567", sender: "+15551234567", text: "ping", id: "X1"})
	if sends != 1 {
		t.Fatalf("dedup failed, sends=%d", sends)
	}
	// non-allowlisted sender does not get a reply.
	ch.dispatch(context.Background(), inbound{chatGUID: "iMessage;-;+19998887777", sender: "+19998887777", text: "ping", id: "X2"})
	if sends != 1 {
		t.Fatalf("non-allowlisted should not reply, sends=%d", sends)
	}
}
