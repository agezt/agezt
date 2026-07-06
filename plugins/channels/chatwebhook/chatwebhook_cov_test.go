// SPDX-License-Identifier: MIT

package chatwebhook

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestName(t *testing.T) {
	if got := New(Config{Kind: "Mattermost"}).Name(); got != "mattermost" {
		t.Fatalf("Name = %q", got)
	}
}

func TestNewDefaults(t *testing.T) {
	c := New(Config{Kind: "mattermost"})
	if c.path != "/mattermost" || c.client == nil {
		t.Fatalf("defaults not applied: %+v", c)
	}
	c2 := New(Config{Kind: "googlechat", Path: "/gc", HTTPClient: &http.Client{}})
	if c2.path != "/gc" {
		t.Fatalf("path override = %q", c2.path)
	}
}

func TestStartNoAddrBlocksUntilCancel(t *testing.T) {
	c := New(Config{Kind: "mattermost"})
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
	c := New(Config{Kind: "mattermost", Addr: "127.0.0.1:39501"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()
	time.Sleep(150 * time.Millisecond)
	resp, err := http.Post("http://127.0.0.1:39501/mattermost", "application/x-www-form-urlencoded", strings.NewReader("text=hi"))
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
	c := New(Config{Kind: "mattermost"})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/mattermost")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestHandleInboundMattermostTokenMismatch(t *testing.T) {
	c := New(Config{Kind: "mattermost", Token: "sek"})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	form := url.Values{"token": {"wrong"}, "user_name": {"bob"}, "text": {"hi"}}
	resp, err := http.Post(srv.URL+"/mattermost", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestHandleInboundGoogleChatTokenMismatch(t *testing.T) {
	c := New(Config{Kind: "googlechat", Token: "sek"})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/googlechat?token=wrong", "application/json", strings.NewReader(`{"type":"MESSAGE"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestHandleInboundMattermostDispatches(t *testing.T) {
	var apiSrv *httptest.Server
	var replyHit bool
	apiSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		replyHit = true
		_, _ = w.Write([]byte("ok"))
	}))
	defer apiSrv.Close()

	done := make(chan struct{}, 1)
	c := New(Config{
		Kind:       "mattermost",
		Token:      "sek",
		WebhookURL: apiSrv.URL,
		Allowlist:  channel.NewAllowlist([]string{"bob"}),
		HTTPClient: apiSrv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			done <- struct{}{}
			return channel.Reply{Text: "pong"}, nil
		},
	})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	form := url.Values{"token": {"sek"}, "user_name": {"bob"}, "channel_name": {"town-square"}, "text": {"!ping hi"}, "trigger_word": {"!ping"}, "post_id": {"P1"}}
	resp, err := http.Post(srv.URL+"/mattermost", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
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
		t.Fatal("reply not posted to webhook")
	}
}

func TestDispatchGuards(t *testing.T) {
	c := New(Config{Kind: "mattermost"})
	c.dispatch(context.Background(), inbound{sender: ""})
	c.dispatch(context.Background(), inbound{sender: "bob", text: "  "})
	c.seenBefore("dup")
	c.dispatch(context.Background(), inbound{sender: "bob", text: "hi", id: "dup"})
}

func TestDispatchNotAllowed(t *testing.T) {
	var called bool
	c := New(Config{
		Kind:      "mattermost",
		Allowlist: channel.NewAllowlist([]string{"other"}),
		Bus:       newBus(t),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			called = true
			return channel.Reply{Text: "x"}, nil
		},
	})
	c.dispatch(context.Background(), inbound{sender: "bob", text: "hi", id: "e1"})
	if called {
		t.Fatal("handler must not run for non-allowlisted sender")
	}
}

func TestDispatchHandlerErrorSendsApology(t *testing.T) {
	var sent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		sent = string(b)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c := New(Config{
		Kind:       "mattermost",
		WebhookURL: srv.URL,
		Allowlist:  channel.NewAllowlist([]string{"bob"}),
		HTTPClient: srv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{}, io.EOF
		},
	})
	c.dispatch(context.Background(), inbound{sender: "bob", target: "ch", text: "hi", id: "e2"})
	time.Sleep(100 * time.Millisecond)
	if !strings.Contains(sent, "sorry") {
		t.Fatalf("expected apology, got %q", sent)
	}
}

func TestDispatchEmptyReplyNoSend(t *testing.T) {
	c := New(Config{
		Kind:      "mattermost",
		Allowlist: channel.NewAllowlist([]string{"bob"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{Text: ""}, nil
		},
	})
	c.dispatch(context.Background(), inbound{sender: "bob", text: "hi", id: "e3"})
}

func TestSendEmptyNoop(t *testing.T) {
	if err := New(Config{Kind: "mattermost"}).Send(context.Background(), channel.Outbound{Text: "  "}); err != nil {
		t.Fatalf("empty send should no-op, got %v", err)
	}
}

func TestSendRequiresWebhook(t *testing.T) {
	if err := New(Config{Kind: "mattermost"}).Send(context.Background(), channel.Outbound{Text: "hi"}); err == nil {
		t.Fatal("expected error without webhook URL")
	}
}

func TestSendChunksAndPublishes(t *testing.T) {
	var hits int
	var lastPayload string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		b, _ := io.ReadAll(r.Body)
		lastPayload = string(b)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	long := strings.Repeat("a", chatMaxChars+50)
	c := New(Config{Kind: "mattermost", WebhookURL: srv.URL, HTTPClient: srv.Client(), Bus: newBus(t)})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "town-square", Text: long}); err != nil {
		t.Fatal(err)
	}
	if hits < 2 {
		t.Fatalf("expected >=2 chunk posts, got %d", hits)
	}
	if !strings.Contains(lastPayload, "town-square") {
		t.Fatalf("mattermost channel override missing: %q", lastPayload)
	}
}

func TestSendNon2xxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	c := New(Config{Kind: "googlechat", WebhookURL: srv.URL, HTTPClient: srv.Client()})
	if err := c.Send(context.Background(), channel.Outbound{Text: "hi"}); err == nil {
		t.Fatal("expected non-2xx error")
	}
}

func TestSendTransportError(t *testing.T) {
	c := New(Config{Kind: "mattermost", WebhookURL: "http://127.0.0.1:0", HTTPClient: &http.Client{Timeout: 200 * time.Millisecond}})
	if err := c.Send(context.Background(), channel.Outbound{Text: "hi"}); err == nil {
		t.Fatal("expected transport error")
	}
}

func TestEmitInboundNilBus(t *testing.T) {
	New(Config{Kind: "mattermost"}).emitInbound(channel.UnifiedMessage{}, "corr", true)
}

func TestEmitInboundWithBus(t *testing.T) {
	c := New(Config{Kind: "mattermost", Bus: newBus(t)})
	c.emitInbound(channel.UnifiedMessage{ChannelID: "ch", Sender: "bob", Text: "hi"}, "corr", true)
}

func TestSeenBeforeRingEviction(t *testing.T) {
	c := New(Config{Kind: "mattermost"})
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

func TestParseInboundBothDialects(t *testing.T) {
	// Mattermost form.
	m, ok := parseInbound(KindMattermost, []byte("user_name=bob&channel_name=ch&text=hi&post_id=P1"))
	if !ok || m.sender != "bob" || m.target != "ch" || m.text != "hi" || m.id != "P1" {
		t.Fatalf("mattermost = %+v ok=%v", m, ok)
	}
	// Google Chat MESSAGE event.
	g, ok := parseInbound(KindGoogleChat, []byte(`{"type":"MESSAGE","message":{"name":"m1","text":"hello","sender":{"displayName":"Al","email":"al@x.com"}},"space":{"name":"spaces/AAA"}}`))
	if !ok || g.text != "hello" {
		t.Fatalf("googlechat = %+v ok=%v", g, ok)
	}
	// Non-MESSAGE google chat event dropped.
	if _, ok := parseInbound(KindGoogleChat, []byte(`{"type":"ADDED_TO_SPACE"}`)); ok {
		t.Fatal("non-MESSAGE event should be dropped")
	}
	// Invalid google chat JSON.
	if _, ok := parseInbound(KindGoogleChat, []byte("not json")); ok {
		t.Fatal("invalid JSON should be dropped")
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
