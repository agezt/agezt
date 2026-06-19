// SPDX-License-Identifier: MIT

package zalo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/channel"
)

func TestParseEvent(t *testing.T) {
	body := []byte(`{"event_name":"user_send_text","sender":{"id":"U1"},"message":{"msg_id":"M1","text":"hi"},"timestamp":"1700"}`)
	m, ts, ok := parseEvent(body)
	if !ok || m.sender != "U1" || m.text != "hi" || m.id != "M1" || ts != "1700" {
		t.Fatalf("parseEvent = %+v ts=%q ok=%v", m, ts, ok)
	}
	// non-text events dropped.
	if _, _, ok := parseEvent([]byte(`{"event_name":"follow","sender":{"id":"U"}}`)); ok {
		t.Fatal("follow should be dropped")
	}
}

func TestValidSignature(t *testing.T) {
	c := New(Config{AppID: "app", Secret: "sek"})
	body := []byte(`{"x":1}`)
	ts := "1700"
	h := sha256.New()
	h.Write([]byte("app"))
	h.Write(body)
	h.Write([]byte(ts))
	h.Write([]byte("sek"))
	mac := "mac=" + hex.EncodeToString(h.Sum(nil))
	if !c.validSignature(body, ts, mac) {
		t.Fatal("valid signature rejected")
	}
	if c.validSignature(body, ts, "mac=deadbeef") {
		t.Fatal("bad signature accepted")
	}
}

func TestSendShape(t *testing.T) {
	var body, token string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token = r.Header.Get("access_token")
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}))
	defer srv.Close()
	c := New(Config{AccessToken: "T", APIBase: srv.URL, HTTPClient: srv.Client()})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "U1", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	if token != "T" {
		t.Fatalf("access_token header = %q", token)
	}
	var p struct {
		Recipient struct {
			UserID string `json:"user_id"`
		} `json:"recipient"`
		Message struct {
			Text string `json:"text"`
		} `json:"message"`
	}
	_ = json.Unmarshal([]byte(body), &p)
	if p.Recipient.UserID != "U1" || p.Message.Text != "hi" {
		t.Fatalf("body = %s", body)
	}
}

func TestDispatchAllowlist(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer srv.Close()
	c := New(Config{
		AccessToken: "T",
		APIBase:     srv.URL,
		Allowlist:   channel.NewAllowlist([]string{"U1"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (string, error) {
			return "pong", nil
		},
		HTTPClient: srv.Client(),
	})
	c.dispatch(context.Background(), inbound{sender: "U1", text: "ping", id: "M1"})
	c.dispatch(context.Background(), inbound{sender: "U1", text: "ping", id: "M1"}) // dedup
	c.dispatch(context.Background(), inbound{sender: "U2", text: "ping", id: "M2"}) // not allowed
	if hits != 1 {
		t.Fatalf("expected 1 send, got %d", hits)
	}
}

func TestFreshTimestamp(t *testing.T) {
	now := time.Now().UnixMilli()
	if !freshTimestamp(strconv.FormatInt(now, 10)) {
		t.Fatal("current timestamp rejected")
	}
	if freshTimestamp("1700000000000") {
		t.Fatal("stale timestamp accepted")
	}
	if freshTimestamp("not-a-number") {
		t.Fatal("garbage timestamp accepted")
	}
}
