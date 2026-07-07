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

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestName(t *testing.T) {
	if got := New(Config{}).Name(); got != "line" {
		t.Fatalf("Name = %q", got)
	}
}

func TestNewDefaults(t *testing.T) {
	c := New(Config{})
	if c.path != DefaultPath || c.apiBase != defaultAPIBase || c.client == nil {
		t.Fatalf("defaults not applied: %+v", c)
	}
	c2 := New(Config{Path: "/l", APIBase: "https://x/", HTTPClient: &http.Client{}})
	if c2.path != "/l" || c2.apiBase != "https://x" {
		t.Fatalf("overrides not applied: %q %q", c2.path, c2.apiBase)
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
	c := New(Config{Addr: "127.0.0.1:39471"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()
	time.Sleep(150 * time.Millisecond)
	resp, err := http.Post("http://127.0.0.1:39471"+DefaultPath, "application/json", strings.NewReader(`{"events":[]}`))
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

func TestHandleInboundBadSignature(t *testing.T) {
	c := New(Config{Secret: "sek"})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+DefaultPath, strings.NewReader(`{"events":[]}`))
	req.Header.Set("X-Line-Signature", "wrong")
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
	var apiSrv *httptest.Server
	var replyHit bool
	apiSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/message/reply") {
			replyHit = true
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer apiSrv.Close()

	secret := "sek"
	done := make(chan struct{}, 1)
	c := New(Config{
		Secret:     secret,
		Allowlist:  channel.NewAllowlist([]string{"U1"}),
		APIBase:    apiSrv.URL,
		HTTPClient: apiSrv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			done <- struct{}{}
			return channel.Reply{Text: "pong"}, nil
		},
	})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()

	body := []byte(`{"events":[{"type":"message","replyToken":"R1","source":{"userId":"U1"},"message":{"type":"text","id":"M1","text":"hi"}}]}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+DefaultPath, strings.NewReader(string(body)))
	req.Header.Set("X-Line-Signature", sign(secret, body))
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
	// Give the async reply a moment.
	time.Sleep(150 * time.Millisecond)
	if !replyHit {
		t.Fatal("reply-token endpoint was not called")
	}
}

func TestDispatchGuards(t *testing.T) {
	c := New(Config{})
	c.dispatch(context.Background(), inbound{userID: ""})
	c.dispatch(context.Background(), inbound{userID: "U1", text: "  "})
	c.seenBefore("dup")
	c.dispatch(context.Background(), inbound{userID: "U1", text: "hi", id: "dup"})
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
	c.dispatch(context.Background(), inbound{userID: "U1", text: "hi", id: "e1"})
	if called {
		t.Fatal("handler must not run for non-allowlisted user")
	}
}

func TestDispatchHandlerErrorFallsBackToPush(t *testing.T) {
	var apiSrv *httptest.Server
	var pushHit bool
	apiSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/message/push") {
			pushHit = true
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer apiSrv.Close()
	c := New(Config{
		Allowlist:  channel.NewAllowlist([]string{"U1"}),
		APIBase:    apiSrv.URL,
		HTTPClient: apiSrv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{}, io.EOF
		},
	})
	// No replyToken => reply routes via push (Send).
	c.dispatch(context.Background(), inbound{userID: "U1", text: "hi", id: "e2"})
	time.Sleep(100 * time.Millisecond)
	if !pushHit {
		t.Fatal("push endpoint not called when replyToken absent")
	}
}

func TestDispatchEmptyReplyNoSend(t *testing.T) {
	c := New(Config{
		Allowlist: channel.NewAllowlist([]string{"U1"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{Text: ""}, nil
		},
	})
	c.dispatch(context.Background(), inbound{userID: "U1", text: "hi", id: "e3", replyToken: "R1"})
}

func TestDispatchFetchesImage(t *testing.T) {
	var apiSrv *httptest.Server
	var gotImage bool
	apiSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/content") {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("PNG"))
			return
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer apiSrv.Close()
	c := New(Config{
		Allowlist:  channel.NewAllowlist([]string{"U1"}),
		APIBase:    apiSrv.URL,
		HTTPClient: apiSrv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			if len(m.Images) == 1 && strings.HasPrefix(m.Images[0], "data:image/png;base64,") {
				gotImage = true
			}
			return channel.Reply{}, nil
		},
	})
	c.dispatch(context.Background(), inbound{userID: "U1", id: "M1", mediaType: "image"})
	if !gotImage {
		t.Fatal("image content not attached")
	}
}

func TestDispatchFetchesAudio(t *testing.T) {
	var apiSrv *httptest.Server
	var gotAudio bool
	apiSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/content") {
			// No content type => defaults to audio/m4a.
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte{1, 2, 3})
			return
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer apiSrv.Close()
	c := New(Config{
		Allowlist:  channel.NewAllowlist([]string{"U1"}),
		APIBase:    apiSrv.URL,
		HTTPClient: apiSrv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			if len(m.Audio) == 1 && strings.HasPrefix(m.Audio[0], "data:") && len(m.Images) == 0 {
				gotAudio = true
			}
			return channel.Reply{}, nil
		},
	})
	c.dispatch(context.Background(), inbound{userID: "U1", id: "M2", mediaType: "audio"})
	if !gotAudio {
		t.Fatal("audio content not attached")
	}
}

func TestFetchContentGuards(t *testing.T) {
	c := New(Config{})
	if got := c.fetchContent(context.Background(), "", "image"); got != "" {
		t.Fatalf("empty id => %q", got)
	}
}

func TestFetchContentNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client()})
	if got := c.fetchContent(context.Background(), "M1", "image"); got != "" {
		t.Fatalf("non-2xx => %q", got)
	}
}

func TestFetchContentEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client()})
	if got := c.fetchContent(context.Background(), "M1", "image"); got != "" {
		t.Fatalf("empty body => %q", got)
	}
}

func TestSendRequiresTarget(t *testing.T) {
	if err := New(Config{}).Send(context.Background(), channel.Outbound{Text: "hi"}); err == nil {
		t.Fatal("expected error without target")
	}
}

func TestSendEmptyTextNoop(t *testing.T) {
	if err := New(Config{}).Send(context.Background(), channel.Outbound{ChannelID: "U1", Text: "  "}); err != nil {
		t.Fatalf("empty text should no-op, got %v", err)
	}
}

func TestSendNon2xxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client()})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "U1", Text: "hi"}); err == nil {
		t.Fatal("expected non-2xx error")
	}
}

func TestSendPublishesToBus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client(), Bus: newBus(t)})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "U1", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
}

func TestSendTransportError(t *testing.T) {
	c := New(Config{APIBase: "http://127.0.0.1:0", HTTPClient: &http.Client{Timeout: 200 * time.Millisecond}})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "U1", Text: "hi"}); err == nil {
		t.Fatal("expected transport error")
	}
}

func TestEmitInboundNilBus(t *testing.T) {
	New(Config{}).emitInbound(channel.UnifiedMessage{}, "corr", true)
}

func TestEmitInboundWithBus(t *testing.T) {
	c := New(Config{Bus: newBus(t)})
	c.emitInbound(channel.UnifiedMessage{ChannelID: "U1", Sender: "U1", Text: "hi"}, "corr", true)
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

func TestParseWebhookMedia(t *testing.T) {
	img := parseWebhook([]byte(`{"events":[{"type":"message","source":{"userId":"U"},"message":{"type":"image","id":"IMG"}}]}`))
	if len(img) != 1 || img[0].mediaType != "image" || img[0].id != "IMG" {
		t.Fatalf("image = %+v", img)
	}
	aud := parseWebhook([]byte(`{"events":[{"type":"message","source":{"groupId":"G"},"message":{"type":"audio","id":"AUD"}}]}`))
	if len(aud) != 1 || aud[0].mediaType != "audio" || aud[0].userID != "G" {
		t.Fatalf("audio = %+v", aud)
	}
	// Invalid JSON => nil.
	if got := parseWebhook([]byte("not json")); got != nil {
		t.Fatalf("invalid JSON => %+v", got)
	}
}

func TestTextMessagesChunks(t *testing.T) {
	long := strings.Repeat("a", lineMaxChars+50)
	msgs := textMessages(long)
	if len(msgs) < 2 {
		t.Fatalf("expected chunking, got %d messages", len(msgs))
	}
	// json round-trip to ensure shape is valid.
	if _, err := json.Marshal(msgs); err != nil {
		t.Fatal(err)
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
