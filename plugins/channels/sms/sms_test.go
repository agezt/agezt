// SPDX-License-Identifier: MIT

package sms

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

const testPublicURL = "https://bot.example.com/sms"

func newTestChannel(t *testing.T, cfg Config) (*Channel, *journal.Journal) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	cfg.Bus = b
	if cfg.PublicURL == "" {
		cfg.PublicURL = testPublicURL
	}
	return New(cfg), j
}

// signedRequest builds an inbound POST with a valid X-Twilio-Signature for form,
// signed against testPublicURL (which the channel uses as the signing URL).
func signedRequest(token string, form url.Values) *http.Request {
	body := form.Encode()
	req := httptest.NewRequest(http.MethodPost, "/sms", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", twilioSignature(token, testPublicURL, form))
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

// A correctly-signed text from an allowlisted number drives the agent and the
// reply comes back as TwiML.
func TestInbound_AllowedRepliesViaTwiML(t *testing.T) {
	var got channel.UnifiedMessage
	h := func(_ context.Context, m channel.UnifiedMessage, _ string) (channel.Reply, error) {
		got = m
		return channel.Reply{Text: "pong"}, nil
	}
	c, j := newTestChannel(t, Config{
		AuthToken: "tok", Allowlist: channel.NewAllowlist([]string{"+15551230001"}), Handler: h,
	})
	form := url.Values{"From": {"+15551230001"}, "Body": {"ping"}, "MessageSid": {"SM1"}}
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, signedRequest("tok", form))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if got.Text != "ping" || got.ChannelID != "+15551230001" || got.Sender != "+15551230001" {
		t.Errorf("handler got unexpected msg: %+v", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<Message>pong</Message>") {
		t.Errorf("TwiML reply = %q, want a <Message>pong</Message>", body)
	}
	if countKind(t, j, event.KindChannelInbound) != 1 {
		t.Error("expected a channel.inbound event")
	}
	if countKind(t, j, event.KindChannelOutbound) != 1 {
		t.Error("expected a channel.outbound event")
	}
}

// A bad signature is rejected (401) and the handler never runs.
func TestInbound_BadSignatureRejected(t *testing.T) {
	ran := false
	h := func(_ context.Context, _ channel.UnifiedMessage, _ string) (channel.Reply, error) {
		ran = true
		return channel.Reply{Text: "x"}, nil
	}
	c, _ := newTestChannel(t, Config{AuthToken: "tok", Allowlist: channel.NewAllowlist([]string{"+1"}), Handler: h})
	form := url.Values{"From": {"+1"}, "Body": {"hi"}}
	req := signedRequest("WRONG-TOKEN", form) // signed with the wrong key
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401", rec.Code)
	}
	if ran {
		t.Error("handler ran for a bad-signature request")
	}
}

// Empty auth token fails closed — no unsigned inbound.
func TestInbound_EmptyTokenFailsClosed(t *testing.T) {
	c, _ := newTestChannel(t, Config{AuthToken: "", Allowlist: channel.NewAllowlist([]string{"+1"})})
	form := url.Values{"From": {"+1"}, "Body": {"hi"}}
	req := httptest.NewRequest(http.MethodPost, "/sms", strings.NewReader(form.Encode()))
	req.Header.Set("X-Twilio-Signature", "anything")
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d want 401 (fail closed)", rec.Code)
	}
}

// A non-allowlisted sender cannot drive the agent: handler never runs, the reply
// is an empty TwiML ack, and the refusal is journaled (allowed=false).
func TestInbound_NotAllowedRefused(t *testing.T) {
	ran := false
	h := func(_ context.Context, _ channel.UnifiedMessage, _ string) (channel.Reply, error) {
		ran = true
		return channel.Reply{Text: "nope"}, nil
	}
	c, j := newTestChannel(t, Config{AuthToken: "tok", Allowlist: channel.NewAllowlist([]string{"+15550000000"}), Handler: h})
	form := url.Values{"From": {"+15559999999"}, "Body": {"drive me"}, "MessageSid": {"SM2"}}
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, signedRequest("tok", form))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if ran {
		t.Error("handler ran for a non-allowlisted sender")
	}
	if strings.Contains(rec.Body.String(), "<Message>") {
		t.Errorf("refused sender should get an empty TwiML, got %q", rec.Body.String())
	}
	if countKind(t, j, event.KindChannelInbound) != 1 {
		t.Error("expected a channel.inbound event recording the refusal")
	}
	if countKind(t, j, event.KindChannelOutbound) != 0 {
		t.Error("a refused message must not emit an outbound event")
	}
}

// A retried MessageSid (Twilio re-POSTs on timeout) drives the agent only once.
func TestInbound_DedupsMessageSid(t *testing.T) {
	var mu sync.Mutex
	runs := 0
	h := func(_ context.Context, _ channel.UnifiedMessage, _ string) (channel.Reply, error) {
		mu.Lock()
		runs++
		mu.Unlock()
		return channel.Reply{Text: "ok"}, nil
	}
	c, _ := newTestChannel(t, Config{AuthToken: "tok", Allowlist: channel.NewAllowlist([]string{"+1"}), Handler: h})
	form := url.Values{"From": {"+1"}, "Body": {"hi"}, "MessageSid": {"SMdup"}}
	for range 2 {
		rec := httptest.NewRecorder()
		c.Handler().ServeHTTP(rec, signedRequest("tok", form))
	}
	if runs != 1 {
		t.Errorf("handler ran %d times for a duplicate MessageSid, want 1", runs)
	}
}

// Send posts to the Twilio Messages API with Basic auth + To/From/Body, and a
// long reply splits into multiple requests.
func TestSend_PostsToTwilioAndChunks(t *testing.T) {
	type sent struct {
		path, to, from, body, auth string
	}
	var mu sync.Mutex
	var reqs []sent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f, _ := url.ParseQuery(string(b))
		mu.Lock()
		reqs = append(reqs, sent{
			path: r.URL.Path, to: f.Get("To"), from: f.Get("From"), body: f.Get("Body"),
			auth: r.Header.Get("Authorization"),
		})
		mu.Unlock()
		w.WriteHeader(201)
		_, _ = io.WriteString(w, `{"sid":"SMx"}`)
	}))
	defer srv.Close()

	c, j := newTestChannel(t, Config{
		AccountSID: "AC123", AuthToken: "tok", From: "+15550000000",
		APIBase: srv.URL, HTTPClient: srv.Client(),
	})

	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "+15551112222", Text: "hi there"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	mu.Lock()
	if len(reqs) != 1 {
		mu.Unlock()
		t.Fatalf("want 1 request, got %d", len(reqs))
	}
	r0 := reqs[0]
	mu.Unlock()
	if !strings.Contains(r0.path, "/Accounts/AC123/Messages.json") {
		t.Errorf("path = %q, want the Messages endpoint", r0.path)
	}
	if r0.to != "+15551112222" || r0.from != "+15550000000" || r0.body != "hi there" {
		t.Errorf("form = %+v, want To/From/Body set", r0)
	}
	if !strings.HasPrefix(r0.auth, "Basic ") {
		t.Errorf("auth = %q, want Basic", r0.auth)
	}
	if countKind(t, j, event.KindChannelOutbound) != 1 {
		t.Error("expected one channel.outbound event")
	}

	// A long body splits into multiple Twilio requests.
	mu.Lock()
	reqs = nil
	mu.Unlock()
	long := strings.Repeat("a", smsMaxChars+50)
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "+1", Text: long}); err != nil {
		t.Fatalf("long send: %v", err)
	}
	mu.Lock()
	n := len(reqs)
	mu.Unlock()
	if n != 2 {
		t.Errorf("long body produced %d requests, want 2", n)
	}
}

func TestSend_EmptyNoopAndUnconfiguredErrors(t *testing.T) {
	c, _ := newTestChannel(t, Config{AuthToken: "tok"}) // no From/SID
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "+1", Text: "   "}); err != nil {
		t.Errorf("empty send should be a no-op, got %v", err)
	}
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "+1", Text: "hello"}); err == nil {
		t.Error("send with no From/credentials should error")
	}
}
