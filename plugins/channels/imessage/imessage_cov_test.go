// SPDX-License-Identifier: MIT

package imessage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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
	if got := New(Config{}).Name(); got != "imessage" {
		t.Fatalf("Name = %q", got)
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
	c := New(Config{Addr: "127.0.0.1:39491"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()
	time.Sleep(150 * time.Millisecond)
	resp, err := http.Post("http://127.0.0.1:39491"+DefaultPath, "application/json", strings.NewReader(`{}`))
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

func TestHandleInboundBadSecret(t *testing.T) {
	c := New(Config{Secret: "sek"})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+DefaultPath, strings.NewReader(`{}`))
	req.Header.Set("X-Webhook-Secret", "wrong")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestHandleInboundDispatchesAndReplies(t *testing.T) {
	secret := "sek"
	var replyHit bool
	var apiSrv *httptest.Server
	apiSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/message/text") {
			replyHit = true
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer apiSrv.Close()

	done := make(chan struct{}, 1)
	c := New(Config{
		Secret:     secret,
		BaseURL:    apiSrv.URL,
		Allowlist:  channel.NewAllowlist([]string{"+15551234"}),
		HTTPClient: apiSrv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			done <- struct{}{}
			return channel.Reply{Text: "pong"}, nil
		},
	})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	body := `{"type":"new-message","data":{"guid":"MSG1","text":"hi","handle":{"address":"+15551234"},"chats":[{"guid":"iMessage;-;+15551234"}]}}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+DefaultPath, strings.NewReader(body))
	req.Header.Set("X-Webhook-Secret", secret)
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
		t.Fatal("reply not sent to BlueBubbles")
	}
}

func TestDispatchGuards(t *testing.T) {
	c := New(Config{})
	c.dispatch(context.Background(), inbound{}) // no target
	c.dispatch(context.Background(), inbound{sender: "s", text: "  "})
	c.seenBefore("dup")
	c.dispatch(context.Background(), inbound{sender: "s", text: "hi", id: "dup"})
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
	c.dispatch(context.Background(), inbound{sender: "s", chatGUID: "g", text: "hi", id: "e1"})
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
		BaseURL:    srv.URL,
		Allowlist:  channel.NewAllowlist([]string{"s"}),
		HTTPClient: srv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{}, io.EOF
		},
	})
	c.dispatch(context.Background(), inbound{sender: "s", chatGUID: "iMessage;-;s", text: "hi", id: "e2"})
	time.Sleep(100 * time.Millisecond)
	if !strings.Contains(sent, "sorry") {
		t.Fatalf("expected apology, got %q", sent)
	}
}

func TestDispatchEmptyReplyNoSend(t *testing.T) {
	c := New(Config{
		Allowlist: channel.NewAllowlist([]string{"s"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{Text: ""}, nil
		},
	})
	c.dispatch(context.Background(), inbound{sender: "s", chatGUID: "g", text: "hi", id: "e3"})
}

func TestDispatchFetchesImageAttachment(t *testing.T) {
	var gotImage bool
	var apiSrv *httptest.Server
	apiSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/download") {
			_, _ = w.Write([]byte("IMGDATA"))
			return
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer apiSrv.Close()
	c := New(Config{
		BaseURL:    apiSrv.URL,
		Allowlist:  channel.NewAllowlist([]string{"s"}),
		HTTPClient: apiSrv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			if len(m.Images) == 1 && strings.HasPrefix(m.Images[0], "data:image/png;base64,") {
				gotImage = true
			}
			return channel.Reply{}, nil
		},
	})
	c.dispatch(context.Background(), inbound{
		sender: "s", chatGUID: "g", id: "e4",
		attachments: []imAttachment{{guid: "A1", mime: "image/png", name: "p.png"}},
	})
	if !gotImage {
		t.Fatal("image attachment not attached")
	}
}

func TestDispatchFetchesAudioAttachment(t *testing.T) {
	var gotAudio bool
	var apiSrv *httptest.Server
	apiSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/download") {
			_, _ = w.Write([]byte("AUDDATA"))
			return
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer apiSrv.Close()
	c := New(Config{
		BaseURL:    apiSrv.URL,
		Allowlist:  channel.NewAllowlist([]string{"s"}),
		HTTPClient: apiSrv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			if len(m.Audio) == 1 && strings.HasPrefix(m.Audio[0], "data:audio/") {
				gotAudio = true
			}
			return channel.Reply{}, nil
		},
	})
	c.dispatch(context.Background(), inbound{
		sender: "s", chatGUID: "g", id: "e5",
		attachments: []imAttachment{{guid: "A2", mime: "audio/mp4", name: "a.m4a"}},
	})
	if !gotAudio {
		t.Fatal("audio attachment not attached")
	}
}

func TestFetchAttachmentDataNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, HTTPClient: srv.Client()})
	if got := c.fetchAttachmentData(context.Background(), imAttachment{guid: "A1", mime: "image/png"}); got != "" {
		t.Fatalf("non-2xx => %q", got)
	}
}

func TestFetchAttachmentDataEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, HTTPClient: srv.Client()})
	if got := c.fetchAttachmentData(context.Background(), imAttachment{guid: "A1", mime: "image/png"}); got != "" {
		t.Fatalf("empty body => %q", got)
	}
}

func TestFetchAttachmentDataDefaultMime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No content type + empty attachment mime => application/octet-stream.
		_, _ = w.Write([]byte("DATA"))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Password: "pw", HTTPClient: srv.Client()})
	got := c.fetchAttachmentData(context.Background(), imAttachment{guid: "A1"})
	if !strings.HasPrefix(got, "data:") {
		t.Fatalf("default-mime data URL not produced: %q", got)
	}
}

func TestSendRequiresTarget(t *testing.T) {
	if err := New(Config{}).Send(context.Background(), channel.Outbound{Text: "hi"}); err == nil {
		t.Fatal("expected error without a target")
	}
}

func TestSendEmptyNoop(t *testing.T) {
	if err := New(Config{}).Send(context.Background(), channel.Outbound{ChannelID: "g"}); err != nil {
		t.Fatalf("empty send should no-op, got %v", err)
	}
}

func TestSendRequiresBaseURL(t *testing.T) {
	c := New(Config{})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "g", Text: "hi"}); err == nil {
		t.Fatal("expected error without BaseURL")
	}
}

func TestSendTextAndAttachmentPublishes(t *testing.T) {
	var textHit, attHit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/message/text"):
			textHit = true
		case strings.Contains(r.URL.Path, "/message/attachment"):
			attHit = true
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Password: "pw", HTTPClient: srv.Client(), Bus: newBus(t)})
	out := channel.Outbound{
		ChannelID:   "+15551234",
		Text:        "hi",
		Attachments: []channel.Attachment{{Filename: "p.png", Data: []byte("PNG"), MIME: "image/png"}},
	}
	if err := c.Send(context.Background(), out); err != nil {
		t.Fatal(err)
	}
	if !textHit || !attHit {
		t.Fatalf("textHit=%v attHit=%v", textHit, attHit)
	}
}

func TestSendOneNon2xxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, HTTPClient: srv.Client()})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "g", Text: "hi"}); err == nil {
		t.Fatal("expected non-2xx send error")
	}
}

func TestSendAttachmentEmptyDataSkipped(t *testing.T) {
	c := New(Config{BaseURL: "http://127.0.0.1:0", HTTPClient: &http.Client{Timeout: 200 * time.Millisecond}})
	// Empty attachment data returns nil without any HTTP call.
	if err := c.sendAttachment(context.Background(), "g", channel.Attachment{}); err != nil {
		t.Fatalf("empty attachment should be skipped, got %v", err)
	}
}

func TestEmitInboundNilBus(t *testing.T) {
	New(Config{}).emitInbound(channel.UnifiedMessage{}, "corr", true)
}

func TestEmitInboundWithBus(t *testing.T) {
	c := New(Config{Bus: newBus(t)})
	c.emitInbound(channel.UnifiedMessage{ChannelID: "g", Sender: "s", Text: "hi"}, "corr", true)
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

func TestParseWebhookDrops(t *testing.T) {
	// Wrong type dropped.
	if _, ok := parseWebhook([]byte(`{"type":"typing-indicator","data":{"text":"x"}}`)); ok {
		t.Fatal("non new-message should be dropped")
	}
	// isFromMe dropped.
	if _, ok := parseWebhook([]byte(`{"type":"new-message","data":{"text":"x","isFromMe":true}}`)); ok {
		t.Fatal("own message should be dropped")
	}
	// invalid JSON.
	if _, ok := parseWebhook([]byte("not json")); ok {
		t.Fatal("invalid JSON should be dropped")
	}
	// attachments with empty guids are skipped.
	m, ok := parseWebhook([]byte(`{"type":"new-message","data":{"text":"hi","handle":{"address":"s"},"attachments":[{"guid":""},{"guid":"A","mimeType":"image/png"}]}}`))
	if !ok || len(m.attachments) != 1 || m.attachments[0].guid != "A" {
		t.Fatalf("attachment filtering = %+v ok=%v", m, ok)
	}
}

func TestChatGUIDWrapping(t *testing.T) {
	if got := chatGUID("iMessage;-;x"); got != "iMessage;-;x" {
		t.Fatalf("full guid mangled: %q", got)
	}
	if got := chatGUID("+15551234"); got != "iMessage;-;+15551234" {
		t.Fatalf("addr not wrapped: %q", got)
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
