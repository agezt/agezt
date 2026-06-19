// SPDX-License-Identifier: MIT

package line

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

func TestParseWebhook(t *testing.T) {
	body := []byte(`{"events":[{"type":"message","replyToken":"R1","source":{"type":"user","userId":"U123"},"message":{"type":"text","id":"M1","text":"hi"}}]}`)
	got := parseWebhook(body)
	if len(got) != 1 || got[0].userID != "U123" || got[0].replyToken != "R1" || got[0].text != "hi" || got[0].id != "M1" {
		t.Fatalf("parseWebhook = %+v", got)
	}
	// non-text and non-message events are dropped.
	if g := parseWebhook([]byte(`{"events":[{"type":"follow","source":{"userId":"U"}},{"type":"message","message":{"type":"sticker"}}]}`)); len(g) != 0 {
		t.Fatalf("expected no text messages, got %+v", g)
	}
}

func TestValidSignature(t *testing.T) {
	secret := "sek"
	body := []byte(`{"events":[]}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if !validSignature(secret, body, sig) {
		t.Fatal("valid signature rejected")
	}
	if validSignature(secret, body, "deadbeef") {
		t.Fatal("bad signature accepted")
	}
}

func TestSendPushShape(t *testing.T) {
	var path, auth, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}))
	defer srv.Close()
	ch := New(Config{AccessToken: "tok", APIBase: srv.URL, HTTPClient: srv.Client()})
	if err := ch.Send(context.Background(), channel.Outbound{ChannelID: "U1", Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	if path != "/v2/bot/message/push" || auth != "Bearer tok" {
		t.Fatalf("path=%q auth=%q", path, auth)
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	if p["to"] != "U1" {
		t.Fatalf("body = %s", body)
	}
}

func TestInboundRepliesViaReplyToken(t *testing.T) {
	var replies int
	var lastToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/bot/message/reply" {
			replies++
			b, _ := io.ReadAll(r.Body)
			var p map[string]any
			_ = json.Unmarshal(b, &p)
			lastToken, _ = p["replyToken"].(string)
		}
	}))
	defer srv.Close()
	ch := New(Config{
		AccessToken: "tok",
		APIBase:     srv.URL,
		Allowlist:   channel.NewAllowlist([]string{"U123"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{Text: "pong"}, nil
		},
		HTTPClient: srv.Client(),
	})
	ch.dispatch(context.Background(), inbound{userID: "U123", replyToken: "R1", text: "ping", id: "M1"})
	if replies != 1 || lastToken != "R1" {
		t.Fatalf("expected reply to R1, replies=%d token=%q", replies, lastToken)
	}
	// dedup by message id.
	ch.dispatch(context.Background(), inbound{userID: "U123", replyToken: "R2", text: "ping", id: "M1"})
	if replies != 1 {
		t.Fatalf("dedup failed, replies=%d", replies)
	}
	// non-allowlisted sender gets no reply.
	ch.dispatch(context.Background(), inbound{userID: "Uxxx", replyToken: "R3", text: "ping", id: "M2"})
	if replies != 1 {
		t.Fatalf("non-allowlisted should not reply, replies=%d", replies)
	}
}
