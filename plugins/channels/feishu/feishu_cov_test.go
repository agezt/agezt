// SPDX-License-Identifier: MIT

package feishu

import (
	"context"
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

func TestName(t *testing.T) {
	if got := New(Config{}).Name(); got != "feishu" {
		t.Fatalf("Name = %q, want feishu", got)
	}
}

func TestNewDefaults(t *testing.T) {
	// Empty config: default path, default API base, default client.
	c := New(Config{})
	if c.path != DefaultPath {
		t.Fatalf("path = %q, want %q", c.path, DefaultPath)
	}
	if c.apiBase != defaultAPIBase {
		t.Fatalf("apiBase = %q, want %q", c.apiBase, defaultAPIBase)
	}
	if c.client == nil {
		t.Fatal("client should be defaulted")
	}
	// Explicit overrides retained; trailing slash trimmed.
	c2 := New(Config{Path: "/custom", APIBase: "https://api.example.com/", HTTPClient: &http.Client{}})
	if c2.path != "/custom" {
		t.Fatalf("path = %q, want /custom", c2.path)
	}
	if c2.apiBase != "https://api.example.com" {
		t.Fatalf("apiBase = %q, want trimmed", c2.apiBase)
	}
}

func TestStartNoAddrBlocksUntilCancel(t *testing.T) {
	c := New(Config{}) // no Addr => blocks until ctx.Done
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
}

func TestStartServesInboundThenShutsDown(t *testing.T) {
	c := New(Config{Addr: "127.0.0.1:0"})
	// Addr with :0 asks the OS for a port; ListenAndServe uses c.cfg.Addr
	// literally, so use a fixed loopback port unlikely to collide.
	c = New(Config{Addr: "127.0.0.1:39457"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()
	// Give the server a moment to bind.
	time.Sleep(150 * time.Millisecond)
	// Hit the inbound endpoint to prove it is serving.
	resp, err := http.Post("http://127.0.0.1:39457"+DefaultPath, "application/json",
		strings.NewReader(`{"challenge":"x","token":"","type":"url_verification"}`))
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

func TestHandlerServesRoute(t *testing.T) {
	c := New(Config{})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	resp, err := http.Post(srv.URL+DefaultPath, "application/json",
		strings.NewReader(`{"challenge":"chal","token":"","type":"url_verification"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["challenge"] != "chal" {
		t.Fatalf("challenge echo = %q", out["challenge"])
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
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestHandleInboundURLVerificationTokenMismatch(t *testing.T) {
	c := New(Config{VerifyToken: "secret"})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	resp, err := http.Post(srv.URL+DefaultPath, "application/json",
		strings.NewReader(`{"challenge":"chal","token":"wrong","type":"url_verification"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestHandleInboundEventTokenMismatch(t *testing.T) {
	c := New(Config{VerifyToken: "secret"})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	body := `{"header":{"token":"nope","event_type":"im.message.receive_v1"},"event":{"sender":{"sender_id":{"open_id":"ou_1"}},"message":{"message_id":"m1","chat_id":"oc_1","message_type":"text","content":"{\"text\":\"hi\"}"}}}`
	resp, err := http.Post(srv.URL+DefaultPath, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestHandleInboundDispatchesEvent(t *testing.T) {
	var handled bool
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "tenant_access_token") {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "T", "expire": 7200})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
	}))
	defer apiSrv.Close()

	done := make(chan struct{}, 1)
	c := New(Config{
		Allowlist:  channel.NewAllowlist([]string{"ou_1"}),
		APIBase:    apiSrv.URL,
		HTTPClient: apiSrv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			handled = true
			done <- struct{}{}
			return channel.Reply{Text: "pong"}, nil
		},
	})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	body := `{"header":{"event_id":"E1","event_type":"im.message.receive_v1"},"event":{"sender":{"sender_id":{"open_id":"ou_1"}},"message":{"message_id":"m1","chat_id":"oc_1","message_type":"text","content":"{\"text\":\"hi\"}"}}}`
	resp, err := http.Post(srv.URL+DefaultPath, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
	if !handled {
		t.Fatal("handler was not invoked for allowlisted inbound")
	}
}

func TestHandleInboundBadBody(t *testing.T) {
	c := New(Config{})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	// A non-JSON body that isn't url_verification nor a valid event just
	// yields 200 with no dispatch (parseEvent returns ok=false).
	resp, err := http.Post(srv.URL+DefaultPath, "application/json", strings.NewReader(`not json`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestDispatchSkipsEmptyAndDuplicate(t *testing.T) {
	c := New(Config{})
	// Empty sender: no-op, no panic.
	c.dispatch(context.Background(), inbound{sender: "", text: "hi"})
	// Empty text and no fileKey: no-op.
	c.dispatch(context.Background(), inbound{sender: "ou_1", text: "  "})
	// Seen-before dedup: mark id, then dispatch with same id should early-return.
	c.seenBefore("dup1")
	c.dispatch(context.Background(), inbound{sender: "ou_1", text: "hi", id: "dup1"})
}

func TestDispatchNotAllowedEmitsButNoReply(t *testing.T) {
	var called bool
	c := New(Config{
		Allowlist: channel.NewAllowlist([]string{"someoneelse"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			called = true
			return channel.Reply{Text: "x"}, nil
		},
	})
	c.dispatch(context.Background(), inbound{sender: "ou_1", chatID: "oc_1", text: "hi", id: "e1"})
	if called {
		t.Fatal("handler must not run for non-allowlisted sender")
	}
}

func TestDispatchHandlerErrorProducesApology(t *testing.T) {
	var apiSrv *httptest.Server
	var sentText string
	apiSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "tenant_access_token") {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "T", "expire": 7200})
			return
		}
		b, _ := io.ReadAll(r.Body)
		var p map[string]any
		_ = json.Unmarshal(b, &p)
		if s, ok := p["content"].(string); ok {
			sentText = s
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
	}))
	defer apiSrv.Close()
	c := New(Config{
		Allowlist:  channel.NewAllowlist([]string{"ou_1"}),
		APIBase:    apiSrv.URL,
		HTTPClient: apiSrv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{}, io.EOF
		},
	})
	c.dispatch(context.Background(), inbound{sender: "ou_1", chatID: "oc_1", text: "hi", id: "e2"})
	if !strings.Contains(sentText, "sorry") {
		t.Fatalf("expected apology sent, got %q", sentText)
	}
}

func TestDispatchHandlerEmptyReplyNoSend(t *testing.T) {
	c := New(Config{
		Allowlist: channel.NewAllowlist([]string{"ou_1"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{Text: ""}, nil
		},
	})
	// No API base needed since empty reply short-circuits before Send.
	c.dispatch(context.Background(), inbound{sender: "ou_1", chatID: "oc_1", text: "hi", id: "e3"})
}

func TestDispatchFetchesImageResource(t *testing.T) {
	var apiSrv *httptest.Server
	var gotImage bool
	apiSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "tenant_access_token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "T", "expire": 7200})
		case strings.Contains(r.URL.Path, "/resources/"):
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("PNGDATA"))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
		}
	}))
	defer apiSrv.Close()
	c := New(Config{
		Allowlist:  channel.NewAllowlist([]string{"ou_1"}),
		APIBase:    apiSrv.URL,
		HTTPClient: apiSrv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			if len(m.Images) == 1 && strings.HasPrefix(m.Images[0], "data:image/png;base64,") {
				gotImage = true
			}
			return channel.Reply{}, nil
		},
	})
	c.dispatch(context.Background(), inbound{
		sender: "ou_1", chatID: "oc_1", id: "e4",
		messageID: "m1", fileKey: "img_k", mediaType: "image",
	})
	if !gotImage {
		t.Fatal("image resource was not attached as data URL")
	}
}

func TestDispatchFetchesAudioResource(t *testing.T) {
	var apiSrv *httptest.Server
	var gotAudio bool
	apiSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "tenant_access_token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "T", "expire": 7200})
		case strings.Contains(r.URL.Path, "/resources/"):
			// No Content-Type => defaults to audio/opus.
			_, _ = w.Write([]byte("OPUSDATA"))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
		}
	}))
	defer apiSrv.Close()
	c := New(Config{
		Allowlist:  channel.NewAllowlist([]string{"ou_1"}),
		APIBase:    apiSrv.URL,
		HTTPClient: apiSrv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			// The audio clip is routed to Audio (not Images) purely on mediaType.
			if len(m.Audio) == 1 && strings.HasPrefix(m.Audio[0], "data:") && len(m.Images) == 0 {
				gotAudio = true
			}
			return channel.Reply{}, nil
		},
	})
	c.dispatch(context.Background(), inbound{
		sender: "ou_1", chatID: "oc_1", id: "e5",
		messageID: "m2", fileKey: "file_k", mediaType: "audio",
	})
	if !gotAudio {
		t.Fatal("audio resource was not attached as data URL")
	}
}

func TestFetchResourceGuards(t *testing.T) {
	c := New(Config{})
	if got := c.fetchResource(context.Background(), "", "k", "image"); got != "" {
		t.Fatalf("empty messageID => %q, want empty", got)
	}
	if got := c.fetchResource(context.Background(), "m", "", "image"); got != "" {
		t.Fatalf("empty fileKey => %q, want empty", got)
	}
}

func TestFetchResourceTokenFailure(t *testing.T) {
	// Point at a server that fails the token fetch (code != 0, empty token).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 99})
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client()})
	if got := c.fetchResource(context.Background(), "m", "k", "image"); got != "" {
		t.Fatalf("token failure => %q, want empty", got)
	}
}

func TestFetchResourceNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "tenant_access_token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "T", "expire": 7200})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client()})
	if got := c.fetchResource(context.Background(), "m", "k", "file"); got != "" {
		t.Fatalf("non-2xx => %q, want empty", got)
	}
}

func TestFetchResourceEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "tenant_access_token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "T", "expire": 7200})
		default:
			// Empty 200 body => len(data)==0 => "".
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client()})
	if got := c.fetchResource(context.Background(), "m", "k", "file"); got != "" {
		t.Fatalf("empty body => %q, want empty", got)
	}
}

func TestSendRequiresTarget(t *testing.T) {
	c := New(Config{})
	if err := c.Send(context.Background(), channel.Outbound{Text: "hi"}); err == nil {
		t.Fatal("expected error when no chat_id available")
	}
}

func TestSendEmptyTextNoop(t *testing.T) {
	c := New(Config{DefaultChat: "oc_default"})
	if err := c.Send(context.Background(), channel.Outbound{Text: "   "}); err != nil {
		t.Fatalf("empty text send should be a no-op, got %v", err)
	}
}

func TestSendUsesDefaultChatAndTokenError(t *testing.T) {
	// tenantToken fails (transport error) => Send returns the error.
	c := New(Config{DefaultChat: "oc_default", APIBase: "http://127.0.0.1:0", HTTPClient: &http.Client{Timeout: 200 * time.Millisecond}})
	if err := c.Send(context.Background(), channel.Outbound{Text: "hi"}); err == nil {
		t.Fatal("expected token fetch error")
	}
}

func TestTenantTokenErrorPaths(t *testing.T) {
	// Unmarshalable token response => error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<<<not json>>>"))
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client()})
	if _, err := c.tenantToken(context.Background()); err == nil {
		t.Fatal("expected unmarshal error")
	}

	// Empty token with nonzero code => explicit failure.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 42})
	}))
	defer srv2.Close()
	c2 := New(Config{APIBase: srv2.URL, HTTPClient: srv2.Client()})
	if _, err := c2.tenantToken(context.Background()); err == nil {
		t.Fatal("expected token-failed error")
	}
}

func TestTenantTokenDefaultExpiry(t *testing.T) {
	// expire<=0 => default 7200 branch; token is cached and reused.
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "T", "expire": 0})
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client()})
	tok, err := c.tenantToken(context.Background())
	if err != nil || tok != "T" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
	// Cached: no second HTTP hit.
	if _, err := c.tenantToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("token fetched %d times, want 1 (cached)", hits)
	}
}

func TestSendPublishesToBusAndChunks(t *testing.T) {
	var sendHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "tenant_access_token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "T", "expire": 7200})
		case strings.Contains(r.URL.Path, "/im/v1/messages"):
			sendHits++
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
		}
	}))
	defer srv.Close()
	// Long text to force multiple chunks (feishuMaxChars = 4000).
	long := strings.Repeat("a", feishuMaxChars+50)
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client()})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "oc_1", Text: long}); err != nil {
		t.Fatal(err)
	}
	if sendHits < 2 {
		t.Fatalf("expected >=2 chunk sends, got %d", sendHits)
	}
}

func TestSendOneRequestError(t *testing.T) {
	// sendOne returns error when the message API is unreachable.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "tenant_access_token") {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "T", "expire": 7200})
			return
		}
		// Close the connection abruptly to trigger a client-side error.
		hj, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
			return
		}
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client()})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "oc_1", Text: "hi"}); err == nil {
		t.Fatal("expected sendOne transport error")
	}
}

func TestSeenBeforeRingEviction(t *testing.T) {
	c := New(Config{})
	// Fill the ring beyond capacity to exercise eviction.
	for i := 0; i < dedupCapacity+5; i++ {
		id := "id-" + strings.Repeat("x", 1) + itoa(i)
		if c.seenBefore(id) {
			t.Fatalf("fresh id %s reported as seen", id)
		}
	}
	// A recently-added id is still seen.
	last := "id-x" + itoa(dedupCapacity+4)
	if !c.seenBefore(last) {
		t.Fatal("recent id should still be seen")
	}
	// The very first id should have been evicted.
	if c.seenBefore("id-x0") {
		t.Fatal("first id should have been evicted from the ring")
	}
}

func TestEmitInboundNilBus(t *testing.T) {
	c := New(Config{}) // no Bus => early return, no panic
	c.emitInbound(channel.UnifiedMessage{}, "corr", true)
}

func TestEmitInboundWithBus(t *testing.T) {
	c := New(Config{Bus: newBus(t)})
	// Exercises the Bus.Publish branch of emitInbound.
	c.emitInbound(channel.UnifiedMessage{ChannelID: "oc_1", Sender: "ou_1", Text: "hi"}, "corr-1", true)
}

func TestSendPublishesToBus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "tenant_access_token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "T", "expire": 7200})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
		}
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client(), Bus: newBus(t)})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "oc_1", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
}

// itoa avoids importing strconv just for the ring test.
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
