// SPDX-License-Identifier: MIT

package whatsapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// graphMock records outbound Graph API message POSTs and returns 200.
type graphMock struct {
	mu   sync.Mutex
	msgs []map[string]any
	auth []string
	srv  *httptest.Server
}

func newGraphMock(t *testing.T) *graphMock {
	g := &graphMock{}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		g.mu.Lock()
		g.msgs = append(g.msgs, m)
		g.auth = append(g.auth, r.Header.Get("Authorization"))
		g.mu.Unlock()
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"messages":[{"id":"wamid.out"}]}`)
	}))
	t.Cleanup(g.srv.Close)
	return g
}
func (g *graphMock) count() int { g.mu.Lock(); defer g.mu.Unlock(); return len(g.msgs) }

func newTestChannel(t *testing.T, cfg Config, g *graphMock) (*Channel, *journal.Journal) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	cfg.Bus = b
	if g != nil {
		cfg.GraphBase = g.srv.URL
		cfg.HTTPClient = g.srv.Client()
	}
	if cfg.AccessToken == "" {
		cfg.AccessToken = "EAAtoken"
	}
	if cfg.PhoneNumberID == "" {
		cfg.PhoneNumberID = "PN123"
	}
	return New(cfg), j
}

func textWebhook(from, id, body string) string {
	return `{"entry":[{"changes":[{"value":{"messages":[{"from":"` + from +
		`","id":"` + id + `","type":"text","text":{"body":"` + body + `"}}]}}]}]}`
}

func signedPost(secret, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/whatsapp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", "sha256="+sign(secret, []byte(body)))
	return req
}

func countKind(t *testing.T, j *journal.Journal, k event.Kind) int {
	t.Helper()
	evs, err := j.Tail(1000)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range evs {
		if e.Kind == k {
			n++
		}
	}
	return n
}

// The GET handshake echoes hub.challenge when the verify token matches.
func TestVerify_HandshakeEchoesChallenge(t *testing.T) {
	c, _ := newTestChannel(t, Config{VerifyToken: "vtok"}, nil)
	req := httptest.NewRequest(http.MethodGet,
		"/whatsapp?hub.mode=subscribe&hub.verify_token=vtok&hub.challenge=42", nil)
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "42" {
		t.Fatalf("handshake = %d %q, want 200 \"42\"", rec.Code, rec.Body.String())
	}
	// Wrong token → 403.
	bad := httptest.NewRequest(http.MethodGet,
		"/whatsapp?hub.mode=subscribe&hub.verify_token=WRONG&hub.challenge=42", nil)
	rec2 := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec2, bad)
	if rec2.Code != http.StatusForbidden {
		t.Errorf("wrong-token handshake = %d, want 403", rec2.Code)
	}
}

// A signed delivery from an allowlisted number drives the agent and the reply is
// POSTed back via the Graph API with a Bearer token.
func TestInbound_AllowedRepliesViaGraph(t *testing.T) {
	g := newGraphMock(t)
	var got channel.UnifiedMessage
	h := func(_ context.Context, m channel.UnifiedMessage, _ string) (string, error) {
		got = m
		return "pong", nil
	}
	c, j := newTestChannel(t, Config{
		AppSecret: "sek", Allowlist: channel.NewAllowlist([]string{"+15551230001"}), Handler: h,
	}, g)
	body := textWebhook("+15551230001", "wamid.1", "ping")
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, signedPost("sek", body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if got.Text != "ping" || got.ChannelID != "+15551230001" {
		t.Errorf("handler got %+v", got)
	}
	if g.count() != 1 {
		t.Fatalf("want 1 Graph send, got %d", g.count())
	}
	g.mu.Lock()
	m, auth := g.msgs[0], g.auth[0]
	g.mu.Unlock()
	if m["to"] != "+15551230001" || m["type"] != "text" {
		t.Errorf("graph msg = %+v, want to/type set", m)
	}
	if txt, _ := m["text"].(map[string]any); txt == nil || txt["body"] != "pong" {
		t.Errorf("graph text body = %+v, want pong", m["text"])
	}
	if !strings.HasPrefix(auth, "Bearer ") {
		t.Errorf("auth = %q, want Bearer", auth)
	}
	if countKind(t, j, event.KindChannelInbound) != 1 {
		t.Error("expected a channel.inbound event")
	}
	if countKind(t, j, event.KindChannelOutbound) != 1 {
		t.Error("expected a channel.outbound event")
	}
}

// A bad signature is rejected and the handler never runs.
func TestInbound_BadSignatureRejected(t *testing.T) {
	g := newGraphMock(t)
	ran := false
	h := func(_ context.Context, _ channel.UnifiedMessage, _ string) (string, error) {
		ran = true
		return "x", nil
	}
	c, _ := newTestChannel(t, Config{AppSecret: "sek", Allowlist: channel.NewAllowlist([]string{"+1"}), Handler: h}, g)
	body := textWebhook("+1", "wamid.2", "hi")
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, signedPost("WRONG", body))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401", rec.Code)
	}
	if ran || g.count() != 0 {
		t.Error("a bad-signature delivery must not run the handler or send")
	}
}

// Empty app secret fails closed.
func TestInbound_EmptySecretFailsClosed(t *testing.T) {
	c, _ := newTestChannel(t, Config{AppSecret: "", Allowlist: channel.NewAllowlist([]string{"+1"})}, nil)
	req := httptest.NewRequest(http.MethodPost, "/whatsapp", strings.NewReader(textWebhook("+1", "x", "hi")))
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d want 401 (fail closed)", rec.Code)
	}
}

// A non-allowlisted sender is refused: no handler, no send, refusal journaled.
func TestInbound_NotAllowedRefused(t *testing.T) {
	g := newGraphMock(t)
	ran := false
	h := func(_ context.Context, _ channel.UnifiedMessage, _ string) (string, error) {
		ran = true
		return "no", nil
	}
	c, j := newTestChannel(t, Config{AppSecret: "sek", Allowlist: channel.NewAllowlist([]string{"+15550000000"}), Handler: h}, g)
	body := textWebhook("+15559999999", "wamid.3", "drive me")
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, signedPost("sek", body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if ran || g.count() != 0 {
		t.Error("a non-allowlisted sender must not run the handler or send")
	}
	if countKind(t, j, event.KindChannelInbound) != 1 {
		t.Error("expected an inbound event recording the refusal")
	}
	if countKind(t, j, event.KindChannelOutbound) != 0 {
		t.Error("a refused message must not emit an outbound event")
	}
}

func TestMessages_ParsesTextAndVoice(t *testing.T) {
	raw := `{"entry":[{"changes":[{"value":{"messages":[
		{"from":"15551234567","id":"m1","type":"text","text":{"body":"hello"}},
		{"from":"15551234567","id":"m2","type":"audio","audio":{"id":"media-abc"}}
	]}}]}]}`
	var wh waWebhook
	if err := json.Unmarshal([]byte(raw), &wh); err != nil {
		t.Fatal(err)
	}
	msgs := wh.messages()
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	if msgs[0].text != "hello" || msgs[0].audioID != "" {
		t.Fatalf("text msg = %+v", msgs[0])
	}
	if msgs[1].audioID != "media-abc" || msgs[1].text != "" {
		t.Fatalf("voice msg = %+v", msgs[1])
	}
}

// A retried delivery (same message id) drives the agent only once.
func TestInbound_DedupsMessageID(t *testing.T) {
	g := newGraphMock(t)
	runs := 0
	h := func(_ context.Context, _ channel.UnifiedMessage, _ string) (string, error) {
		runs++
		return "ok", nil
	}
	c, _ := newTestChannel(t, Config{AppSecret: "sek", Allowlist: channel.NewAllowlist([]string{"+1"}), Handler: h}, g)
	body := textWebhook("+1", "wamid.dup", "hi")
	for range 2 {
		rec := httptest.NewRecorder()
		c.Handler().ServeHTTP(rec, signedPost("sek", body))
	}
	if runs != 1 {
		t.Errorf("handler ran %d times for a duplicate id, want 1", runs)
	}
}

// Send posts to the Graph API and splits a long body into multiple messages.
func TestSend_PostsAndChunks(t *testing.T) {
	g := newGraphMock(t)
	c, _ := newTestChannel(t, Config{AppSecret: "sek"}, g)
	long := strings.Repeat("a", waMaxChars+50)
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "+1", Text: long}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if g.count() != 2 {
		t.Errorf("long body produced %d sends, want 2", g.count())
	}
}

func TestSend_EmptyNoopAndUnconfiguredErrors(t *testing.T) {
	c, _ := newTestChannel(t, Config{AccessToken: "", PhoneNumberID: ""}, nil)
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "+1", Text: "  "}); err != nil {
		t.Errorf("empty send should be a no-op, got %v", err)
	}
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "+1", Text: "hi"}); err == nil {
		t.Error("send with no credentials should error")
	}
}
