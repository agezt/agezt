// SPDX-License-Identifier: MIT

package dingtalk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/journal"
)

func newBus(t *testing.T) *bus.Bus {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	return b
}

func signNow(secret string) (ts, sign string) {
	ts = strconv.FormatInt(time.Now().UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "\n" + secret))
	sign = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return
}

func TestName(t *testing.T) {
	if got := New(Config{}).Name(); got != "dingtalk" {
		t.Fatalf("Name = %q", got)
	}
}

func TestNewDefaults(t *testing.T) {
	c := New(Config{})
	if c.path != DefaultPath || c.client == nil {
		t.Fatalf("defaults not applied: %+v", c)
	}
	c2 := New(Config{Path: "/dt", HTTPClient: &http.Client{}})
	if c2.path != "/dt" {
		t.Fatalf("path override = %q", c2.path)
	}
}

func TestStartNoAddrBlocksUntilCancel(t *testing.T) {
	c := New(Config{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return")
	}
}

func TestStartServesThenShutsDown(t *testing.T) {
	c := New(Config{Addr: "127.0.0.1:39481"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()
	time.Sleep(150 * time.Millisecond)
	resp, err := http.Post("http://127.0.0.1:39481"+DefaultPath, "application/json", strings.NewReader(`{}`))
	if err == nil {
		resp.Body.Close()
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
}

func TestHandleInboundMethodNotAllowed(t *testing.T) {
	c := New(Config{})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + DefaultPath)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestHandleInboundBadSign(t *testing.T) {
	c := New(Config{Secret: "sek"})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+DefaultPath, strings.NewReader(`{}`))
	req.Header.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	req.Header.Set("sign", "wrong")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestHandleInboundDispatchesAndRepliesViaSessionWebhook(t *testing.T) {
	secret := "sek"
	var replyHit bool
	// The reply session webhook (must be a *.dingtalk.com host to be trusted,
	// so we can't point it at httptest). Instead assert the handler runs and
	// the reply is attempted to the configured (trusted) robot webhook.
	robotSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		replyHit = true
		_, _ = w.Write([]byte("{}"))
	}))
	defer robotSrv.Close()

	done := make(chan struct{}, 1)
	c := New(Config{
		Secret:     secret,
		WebhookURL: robotSrv.URL,
		Allowlist:  channel.NewAllowlist([]string{"S1"}),
		HTTPClient: robotSrv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			done <- struct{}{}
			return channel.Reply{Text: "pong"}, nil
		},
	})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	ts, sign := signNow(secret)
	// replyURL points to a non-dingtalk host => safeReplyURL rejects it =>
	// reply falls back to the configured robot webhook.
	body := `{"msgtype":"text","text":{"content":"hi"},"senderStaffId":"S1","msgId":"M1","sessionWebhook":"https://evil.example/x"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+DefaultPath, strings.NewReader(body))
	req.Header.Set("timestamp", ts)
	req.Header.Set("sign", sign)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler not invoked")
	}
	time.Sleep(150 * time.Millisecond)
	if !replyHit {
		t.Fatal("reply not sent to configured robot webhook fallback")
	}
}

func TestDispatchGuards(t *testing.T) {
	c := New(Config{})
	c.dispatch(context.Background(), inbound{sender: ""})
	c.dispatch(context.Background(), inbound{sender: "S1", text: "  "})
	c.seenBefore("dup")
	c.dispatch(context.Background(), inbound{sender: "S1", text: "hi", id: "dup"})
}

func TestDispatchNotAllowed(t *testing.T) {
	var called bool
	c := New(Config{
		Allowlist: channel.NewAllowlist([]string{"other"}),
		Bus:       newBus(t),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			called = true
			return channel.Reply{Text: "x"}, nil
		},
	})
	c.dispatch(context.Background(), inbound{sender: "S1", text: "hi", id: "e1"})
	if called {
		t.Fatal("handler must not run for non-allowlisted sender")
	}
}

func TestDispatchHandlerErrorSendsApology(t *testing.T) {
	var sent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		sent = string(b)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()
	c := New(Config{
		WebhookURL: srv.URL,
		Allowlist:  channel.NewAllowlist([]string{"S1"}),
		HTTPClient: srv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{}, io.EOF
		},
	})
	c.dispatch(context.Background(), inbound{sender: "S1", text: "hi", id: "e2"})
	time.Sleep(100 * time.Millisecond)
	if !strings.Contains(sent, "sorry") {
		t.Fatalf("expected apology, got %q", sent)
	}
}

func TestDispatchEmptyReplyNoSend(t *testing.T) {
	c := New(Config{
		Allowlist: channel.NewAllowlist([]string{"S1"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{Text: ""}, nil
		},
	})
	c.dispatch(context.Background(), inbound{sender: "S1", text: "hi", id: "e3"})
}

func TestSendEmptyTextNoop(t *testing.T) {
	if err := New(Config{}).Send(context.Background(), channel.Outbound{Text: "  "}); err != nil {
		t.Fatalf("empty text should no-op, got %v", err)
	}
}

func TestSendRequiresWebhook(t *testing.T) {
	if err := New(Config{}).Send(context.Background(), channel.Outbound{Text: "hi"}); err == nil {
		t.Fatal("expected error without webhook URL")
	}
}

func TestSendChunksAndPublishes(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()
	long := strings.Repeat("a", dtMaxChars+50)
	c := New(Config{WebhookURL: srv.URL, HTTPClient: srv.Client(), Bus: newBus(t)})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "S1", Text: long}); err != nil {
		t.Fatal(err)
	}
	if hits < 2 {
		t.Fatalf("expected >=2 chunk posts, got %d", hits)
	}
}

func TestSendNon2xxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	c := New(Config{WebhookURL: srv.URL, HTTPClient: srv.Client()})
	if err := c.Send(context.Background(), channel.Outbound{Text: "hi"}); err == nil {
		t.Fatal("expected non-2xx error")
	}
}

func TestPostEmptyURLError(t *testing.T) {
	c := New(Config{})
	if err := c.post(context.Background(), "", "hi"); err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestPostTransportError(t *testing.T) {
	c := New(Config{HTTPClient: &http.Client{Timeout: 200 * time.Millisecond}})
	if err := c.post(context.Background(), "http://127.0.0.1:0/x", "hi"); err == nil {
		t.Fatal("expected transport error")
	}
}

func TestEmitInboundNilBus(t *testing.T) {
	New(Config{}).emitInbound(channel.UnifiedMessage{}, "corr", true)
}

func TestEmitInboundWithBus(t *testing.T) {
	c := New(Config{Bus: newBus(t)})
	c.emitInbound(channel.UnifiedMessage{ChannelID: "S1", Sender: "S1", Text: "hi"}, "corr", true)
}

func TestSeenBeforeRingEviction(t *testing.T) {
	c := New(Config{})
	for i := 0; i < dedupCapacity+5; i++ {
		if c.seenBefore("id" + itoa(i)) {
			t.Fatalf("fresh id%d seen", i)
		}
	}
	if !c.seenBefore("id" + itoa(dedupCapacity+4)) {
		t.Fatal("recent id should be seen")
	}
	if c.seenBefore("id0") {
		t.Fatal("first id should be evicted")
	}
}

func TestValidSignEmptyFields(t *testing.T) {
	if validSign("s", "", "sign") {
		t.Fatal("empty timestamp accepted")
	}
	if validSign("s", "123", "") {
		t.Fatal("empty sign accepted")
	}
	if validSign("s", "not-a-number", "sign") {
		t.Fatal("non-numeric timestamp accepted")
	}
}

func TestParseInboundDrops(t *testing.T) {
	// Non-text message type is dropped.
	if _, ok := parseInbound([]byte(`{"msgtype":"image"}`)); ok {
		t.Fatal("image msgtype should be dropped")
	}
	// Invalid JSON.
	if _, ok := parseInbound([]byte("not json")); ok {
		t.Fatal("invalid JSON should return ok=false")
	}
	// Fallback to senderNick when staff id is empty.
	m, ok := parseInbound([]byte(`{"text":{"content":"hi"},"senderNick":"Bob","msgId":"M"}`))
	if !ok || m.sender != "Bob" {
		t.Fatalf("nick fallback = %+v ok=%v", m, ok)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
