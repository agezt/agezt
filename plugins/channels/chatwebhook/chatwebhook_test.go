// SPDX-License-Identifier: MIT

package chatwebhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

func TestParseMattermost(t *testing.T) {
	form := url.Values{
		"token":        {"tok"},
		"user_name":    {"alice"},
		"channel_name": {"town-square"},
		"post_id":      {"P1"},
		"trigger_word": {"agezt"},
		"text":         {"agezt hello there"},
	}
	m, ok := parseMattermost([]byte(form.Encode()))
	if !ok || m.sender != "alice" || m.target != "town-square" || m.text != "hello there" || m.id != "P1" {
		t.Fatalf("parseMattermost = %+v ok=%v", m, ok)
	}
}

func TestParseGoogleChat(t *testing.T) {
	body := []byte(`{"type":"MESSAGE","message":{"name":"spaces/A/messages/B","text":"hi","sender":{"email":"a@b.com","displayName":"Alice"}},"space":{"name":"spaces/A"}}`)
	m, ok := parseGoogleChat(body)
	if !ok || m.sender != "a@b.com" || m.target != "spaces/A" || m.text != "hi" || m.id != "spaces/A/messages/B" {
		t.Fatalf("parseGoogleChat = %+v ok=%v", m, ok)
	}
	// non-MESSAGE events are dropped.
	if _, ok := parseGoogleChat([]byte(`{"type":"ADDED_TO_SPACE"}`)); ok {
		t.Fatal("ADDED_TO_SPACE should be ignored")
	}
}

func TestVerifyMattermostToken(t *testing.T) {
	c := New(Config{Kind: KindMattermost, Token: "tok"})
	good := []byte(url.Values{"token": {"tok"}, "text": {"x"}}.Encode())
	bad := []byte(url.Values{"token": {"nope"}, "text": {"x"}}.Encode())
	if !c.verify(&http.Request{}, good) {
		t.Fatal("valid token rejected")
	}
	if c.verify(&http.Request{}, bad) {
		t.Fatal("bad token accepted")
	}
}

func TestMattermostReplyShape(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
	}))
	defer srv.Close()
	c := New(Config{
		Kind:       KindMattermost,
		WebhookURL: srv.URL,
		Allowlist:  channel.NewAllowlist([]string{"alice"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{Text: "pong"}, nil
		},
		HTTPClient: srv.Client(),
	})
	c.dispatch(context.Background(), inbound{sender: "alice", target: "town-square", text: "ping", id: "P1"})
	if len(bodies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(bodies))
	}
	var p map[string]any
	_ = json.Unmarshal([]byte(bodies[0]), &p)
	if p["text"] != "pong" || p["channel"] != "town-square" {
		t.Fatalf("reply body = %s", bodies[0])
	}
	// dedup + allowlist.
	c.dispatch(context.Background(), inbound{sender: "alice", target: "town-square", text: "ping", id: "P1"})
	c.dispatch(context.Background(), inbound{sender: "mallory", target: "town-square", text: "ping", id: "P2"})
	if len(bodies) != 1 {
		t.Fatalf("dedup/allowlist failed, bodies=%d", len(bodies))
	}
}

func TestGoogleChatNoChannelOverride(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}))
	defer srv.Close()
	c := New(Config{Kind: KindGoogleChat, WebhookURL: srv.URL, HTTPClient: srv.Client()})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "spaces/A", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"text":"hi"`) || strings.Contains(body, "channel") {
		t.Fatalf("google chat must not set channel override: %s", body)
	}
}
