// SPDX-License-Identifier: MIT

package dingtalk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/channel"
)

func TestParseInbound(t *testing.T) {
	body := []byte(`{"msgtype":"text","text":{"content":"  hello "},"senderStaffId":"S1","senderNick":"Al","msgId":"M1","sessionWebhook":"http://x/reply"}`)
	m, ok := parseInbound(body)
	if !ok || m.sender != "S1" || m.text != "hello" || m.id != "M1" || m.replyURL != "http://x/reply" {
		t.Fatalf("parseInbound = %+v ok=%v", m, ok)
	}
}

func TestValidSign(t *testing.T) {
	secret := "sek"
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "\n" + secret))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if !validSign(secret, ts, sign) {
		t.Fatal("valid sign rejected")
	}
	if validSign(secret, ts, "nope") {
		t.Fatal("bad sign accepted")
	}
	// stale timestamp is rejected even with a correct mac (replay guard).
	stale := "1700000000000"
	mac2 := hmac.New(sha256.New, []byte(secret))
	mac2.Write([]byte(stale + "\n" + secret))
	if validSign(secret, stale, base64.StdEncoding.EncodeToString(mac2.Sum(nil))) {
		t.Fatal("stale timestamp accepted")
	}
}

func TestSafeReplyURL(t *testing.T) {
	if !safeReplyURL("https://oapi.dingtalk.com/robot/sendBySession?session=x") {
		t.Fatal("genuine sessionWebhook rejected")
	}
	for _, bad := range []string{"http://oapi.dingtalk.com/x", "https://evil.com/x", "https://169.254.169.254/x", ""} {
		if safeReplyURL(bad) {
			t.Fatalf("unsafe url accepted: %q", bad)
		}
	}
}

func TestDispatchRepliesToConfiguredWebhook(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
	}))
	defer srv.Close()
	ch := New(Config{
		WebhookURL: srv.URL, // trusted fallback (a non-dingtalk replyURL is refused)
		Allowlist:  channel.NewAllowlist([]string{"S1"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{Text: "pong"}, nil
		},
		HTTPClient: srv.Client(),
	})
	// replyURL points at a non-dingtalk host → refused → falls back to WebhookURL.
	ch.dispatch(context.Background(), inbound{sender: "S1", text: "ping", id: "M1", replyURL: srv.URL})
	var p map[string]any
	_ = json.Unmarshal([]byte(got), &p)
	if p["msgtype"] != "text" {
		t.Fatalf("reply = %s", got)
	}
	// non-allowlisted gets no reply (got unchanged).
	got = ""
	ch.dispatch(context.Background(), inbound{sender: "Sx", text: "ping", id: "M2"})
	if got != "" {
		t.Fatalf("non-allowlisted replied: %s", got)
	}
}
