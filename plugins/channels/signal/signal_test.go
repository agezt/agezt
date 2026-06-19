// SPDX-License-Identifier: MIT

package signal

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

// fakeAPI records POST /v2/send and serves scripted GET /v1/receive responses,
// mirroring a signal-cli-rest-api server.
type fakeAPI struct {
	mu          sync.Mutex
	sent        []sentMsg    // captured outbound /v2/send bodies
	auth        []string     // Authorization header per request
	recvBatches [][]envelope // one per /v1/receive call (last repeats)
	recvCalls   int
	lastTimeout string // the `timeout` query of the most recent /v1/receive
}

type sentMsg struct {
	number     string
	message    string
	recipients []string
}

func (f *fakeAPI) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		f.mu.Unlock()
		p := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/v2/send"):
			body, _ := io.ReadAll(r.Body)
			var m struct {
				Message    string   `json:"message"`
				Number     string   `json:"number"`
				Recipients []string `json:"recipients"`
			}
			_ = json.Unmarshal(body, &m)
			f.mu.Lock()
			f.sent = append(f.sent, sentMsg{number: m.Number, message: m.Message, recipients: m.Recipients})
			f.mu.Unlock()
			_, _ = io.WriteString(w, `{"timestamp":1700000000000}`)
		case r.Method == http.MethodGet && strings.Contains(p, "/v1/receive/"):
			f.mu.Lock()
			f.lastTimeout = r.URL.Query().Get("timeout")
			var out []envelope
			if f.recvCalls < len(f.recvBatches) {
				out = f.recvBatches[f.recvCalls]
			} else if len(f.recvBatches) > 0 {
				out = f.recvBatches[len(f.recvBatches)-1]
			}
			f.recvCalls++
			f.mu.Unlock()
			if out == nil {
				out = []envelope{}
			}
			_ = json.NewEncoder(w).Encode(out)
		default:
			w.WriteHeader(404)
		}
	}
}

func (f *fakeAPI) sentCount() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.sent) }

func newTestChannel(t *testing.T, srv *httptest.Server, token string, allow channel.Allowlist, h channel.InboundHandler) (*Channel, *journal.Journal) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	c := New(Config{
		APIURL:     srv.URL,
		Number:     "+15550000000",
		Token:      token,
		Allowlist:  allow,
		Bus:        b,
		Handler:    h,
		HTTPClient: srv.Client(),
	})
	return c, j
}

func textEnvelope(source, message string) envelope {
	var ev envelope
	ev.Envelope.Source = source
	ev.Envelope.Timestamp = 1700000000000
	ev.Envelope.DataMessage.Message = message
	ev.Envelope.DataMessage.Timestamp = 1700000000000
	return ev
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

// An allowlisted sender's text drives the agent and the reply is POSTed back to
// that sender via /v2/send, carrying the bot's number and the bearer token.
func TestHandleInbound_AllowedRepliesViaSend(t *testing.T) {
	fa := &fakeAPI{}
	srv := httptest.NewServer(fa.handler())
	defer srv.Close()

	var gotMsg channel.UnifiedMessage
	h := func(_ context.Context, m channel.UnifiedMessage, _ string) (channel.Reply, error) {
		gotMsg = m
		return channel.Reply{Text: "pong"}, nil
	}
	c, j := newTestChannel(t, srv, "proxytoken", channel.NewAllowlist([]string{"+15551112222"}), h)
	c.handleInbound(context.Background(), textEnvelope("+15551112222", "ping"))

	if gotMsg.Text != "ping" || gotMsg.ChannelID != "+15551112222" || gotMsg.Sender != "+15551112222" {
		t.Errorf("handler got unexpected msg: %+v", gotMsg)
	}
	if fa.sentCount() != 1 {
		t.Fatalf("want 1 reply POST, got %d", fa.sentCount())
	}
	got := fa.sent[0]
	if got.message != "pong" || got.number != "+15550000000" {
		t.Errorf("reply = %+v, want message pong from +15550000000", got)
	}
	if len(got.recipients) != 1 || got.recipients[0] != "+15551112222" {
		t.Errorf("recipients = %v, want [+15551112222]", got.recipients)
	}
	if a := fa.auth[len(fa.auth)-1]; a != "Bearer proxytoken" {
		t.Errorf("auth header = %q, want bearer token", a)
	}
	if countKind(t, j, event.KindChannelInbound) != 1 {
		t.Error("expected a channel.inbound event")
	}
	if countKind(t, j, event.KindChannelOutbound) != 1 {
		t.Error("expected a channel.outbound event")
	}
}

// A non-allowlisted sender cannot drive the agent: the handler never runs and the
// sender is told once, fail-closed.
func TestHandleInbound_NotAllowedRefused(t *testing.T) {
	fa := &fakeAPI{}
	srv := httptest.NewServer(fa.handler())
	defer srv.Close()

	ran := false
	h := func(_ context.Context, _ channel.UnifiedMessage, _ string) (channel.Reply, error) {
		ran = true
		return channel.Reply{Text: "should not run"}, nil
	}
	c, _ := newTestChannel(t, srv, "", channel.NewAllowlist([]string{"+15559999999"}), h)
	c.handleInbound(context.Background(), textEnvelope("+15551112222", "drive me"))

	if ran {
		t.Error("handler ran for a non-allowlisted sender")
	}
	if fa.sentCount() != 1 || !strings.Contains(fa.sent[0].message, "not authorized") {
		t.Errorf("want one 'not authorized' notice, got %+v", fa.sent)
	}
	// With no token configured, no Authorization header is sent.
	if a := fa.auth[len(fa.auth)-1]; a != "" {
		t.Errorf("auth header = %q, want empty (no token configured)", a)
	}
}

// dispatchable gates the poll loop: only inbound text from a non-self sender.
func TestDispatchable(t *testing.T) {
	c := &Channel{number: "+15550000000"}
	if !c.dispatchable(textEnvelope("+15551112222", "hi")) {
		t.Error("a valid inbound text message should dispatch")
	}
	if c.dispatchable(textEnvelope("+15550000000", "my own reply")) {
		t.Error("the account's OWN message must be skipped (else it loops)")
	}
	if c.dispatchable(textEnvelope("+15551112222", "   ")) {
		t.Error("a whitespace-only message must be skipped")
	}
	// A receipt/typing envelope has no dataMessage text.
	if c.dispatchable(textEnvelope("+15551112222", "")) {
		t.Error("an empty (non-data) message must be skipped")
	}
	if c.dispatchable(textEnvelope("", "x")) {
		t.Error("an envelope with no source must be skipped")
	}
}

// receive parses the envelope array and forwards the poll timeout.
func TestReceive_ParsesAndForwardsTimeout(t *testing.T) {
	fa := &fakeAPI{recvBatches: [][]envelope{{textEnvelope("+15551112222", "hello")}}}
	srv := httptest.NewServer(fa.handler())
	defer srv.Close()
	c, _ := newTestChannel(t, srv, "", channel.NewAllowlist(nil), nil)
	c.pollSecs = 7

	got, err := c.receive(context.Background())
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if len(got) != 1 || got[0].Envelope.DataMessage.Message != "hello" {
		t.Errorf("parsed = %+v, want one 'hello'", got)
	}
	if fa.lastTimeout != "7" {
		t.Errorf("timeout param = %q, want 7", fa.lastTimeout)
	}
}

// send is a no-op for empty text and chunks a long body into multiple POSTs.
func TestSend_EmptyNoopAndChunks(t *testing.T) {
	fa := &fakeAPI{}
	srv := httptest.NewServer(fa.handler())
	defer srv.Close()
	c, _ := newTestChannel(t, srv, "", channel.NewAllowlist(nil), nil)

	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "+15551112222", Text: "   "}); err != nil {
		t.Fatalf("empty send: %v", err)
	}
	if fa.sentCount() != 0 {
		t.Errorf("empty text must not POST; got %d", fa.sentCount())
	}

	long := strings.Repeat("a", signalMaxChars+100) // forces a 2-chunk split
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "+15551112222", Text: long}); err != nil {
		t.Fatalf("long send: %v", err)
	}
	if fa.sentCount() != 2 {
		t.Errorf("long text should split into 2 POSTs, got %d", fa.sentCount())
	}
}

// A failed send (non-2xx) surfaces as an error.
func TestSend_ErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c, _ := newTestChannel(t, srv, "", channel.NewAllowlist(nil), nil)
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "+1", Text: "x"}); err == nil {
		t.Error("a non-2xx send must return an error")
	}
}
