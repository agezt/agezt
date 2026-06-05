// SPDX-License-Identifier: MIT

package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// fakeBotServer records sendMessage calls and serves scripted getUpdates.
type fakeBotServer struct {
	mu       sync.Mutex
	sent     []map[string]any
	updates  [][]tgUpdate // one batch per getUpdates call
	getCalls int
}

func (f *fakeBotServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getUpdates"):
			f.mu.Lock()
			var batch []tgUpdate
			if f.getCalls < len(f.updates) {
				batch = f.updates[f.getCalls]
			}
			f.getCalls++
			f.mu.Unlock()
			json.NewEncoder(w).Encode(getUpdatesResp{OK: true, Result: batch})
		case strings.Contains(r.URL.Path, "/sendMessage"):
			body, _ := io.ReadAll(r.Body)
			var m map[string]any
			json.Unmarshal(body, &m)
			f.mu.Lock()
			f.sent = append(f.sent, m)
			f.mu.Unlock()
			w.WriteHeader(200)
			io.WriteString(w, `{"ok":true}`)
		default:
			w.WriteHeader(404)
		}
	}
}

func (f *fakeBotServer) sentCount() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.sent) }

func newTestChannel(t *testing.T, srv *httptest.Server, allow channel.Allowlist, h channel.InboundHandler) (*Channel, *journal.Journal) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	return New(Config{
		Token:      "TESTTOKEN",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
		Allowlist:  allow,
		Bus:        b,
		Handler:    h,
	}), j
}

func countKind(t *testing.T, j *journal.Journal, k event.Kind) int {
	t.Helper()
	n := 0
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == k {
			n++
		}
		return nil
	})
	return n
}

func TestInboundRunsHandlerAndReplies(t *testing.T) {
	fb := &fakeBotServer{}
	srv := httptest.NewServer(fb.handler())
	defer srv.Close()

	var gotText, gotCorr string
	h := func(_ context.Context, msg channel.UnifiedMessage, corr string) (string, error) {
		gotText = msg.Text
		gotCorr = corr
		return "the answer", nil
	}
	c, j := newTestChannel(t, srv, channel.NewAllowlist([]string{"42"}), h)

	c.handleInbound(context.Background(), &tgMessage{MessageID: 1, Chat: tgChat{ID: 42}, From: &tgUser{Username: "ersin"}, Text: "hi bot"})

	if gotText != "hi bot" {
		t.Fatalf("handler text = %q", gotText)
	}
	if gotCorr == "" {
		t.Fatal("handler should receive a correlation")
	}
	if fb.sentCount() != 1 || fb.sent[0]["text"] != "the answer" {
		t.Fatalf("expected one reply with the answer, got %+v", fb.sent)
	}
	if countKind(t, j, event.KindChannelInbound) != 1 || countKind(t, j, event.KindChannelOutbound) != 1 {
		t.Fatal("inbound and outbound must both be journaled")
	}

	// inbound + outbound share the run correlation → linkable in the inbox.
	var inCorr, outCorr string
	_ = j.Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindChannelInbound:
			inCorr = e.CorrelationID
		case event.KindChannelOutbound:
			outCorr = e.CorrelationID
		}
		return nil
	})
	if inCorr == "" || inCorr != outCorr || inCorr != gotCorr {
		t.Fatalf("inbound/outbound/handler correlations must match: %q %q %q", inCorr, outCorr, gotCorr)
	}
}

func TestInboundAllowlistRejection(t *testing.T) {
	fb := &fakeBotServer{}
	srv := httptest.NewServer(fb.handler())
	defer srv.Close()

	called := false
	h := func(context.Context, channel.UnifiedMessage, string) (string, error) {
		called = true
		return "should not run", nil
	}
	c, j := newTestChannel(t, srv, channel.NewAllowlist([]string{"42"}), h)

	c.handleInbound(context.Background(), &tgMessage{Chat: tgChat{ID: 999}, Text: "let me in"})

	if called {
		t.Fatal("non-allowlisted sender must NOT drive the agent")
	}
	if fb.sentCount() != 1 || fb.sent[0]["text"] != "not authorized" {
		t.Fatalf("rejected sender should get a 'not authorized' notice, got %+v", fb.sent)
	}
	// The inbound is still journaled (with allowed=false) for audit.
	if countKind(t, j, event.KindChannelInbound) != 1 {
		t.Fatal("rejected inbound must still be journaled")
	}
}

func TestSendEmitsOutbound(t *testing.T) {
	fb := &fakeBotServer{}
	srv := httptest.NewServer(fb.handler())
	defer srv.Close()
	c, j := newTestChannel(t, srv, channel.Allowlist{}, nil)

	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "7", Text: "brief"}); err != nil {
		t.Fatal(err)
	}
	if fb.sentCount() != 1 || fb.sent[0]["chat_id"] != "7" {
		t.Fatalf("send should POST sendMessage, got %+v", fb.sent)
	}
	if countKind(t, j, event.KindChannelOutbound) != 1 {
		t.Fatal("Send must journal channel.outbound")
	}
}

func TestStartPollsAndAdvancesOffset(t *testing.T) {
	fb := &fakeBotServer{updates: [][]tgUpdate{
		{{UpdateID: 100, Message: &tgMessage{Chat: tgChat{ID: 42}, Text: "first"}}},
	}}
	srv := httptest.NewServer(fb.handler())
	defer srv.Close()

	var mu sync.Mutex
	var seen []string
	h := func(_ context.Context, msg channel.UnifiedMessage, _ string) (string, error) {
		mu.Lock()
		seen = append(seen, msg.Text)
		mu.Unlock()
		return "", nil
	}
	c, _ := newTestChannel(t, srv, channel.NewAllowlist([]string{"42"}), h)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Start(ctx); close(done) }()

	// Wait until the first update is processed, then cancel.
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(seen) == 1 })
	if c.offset != 101 {
		t.Errorf("offset should advance to update_id+1 (101), got %d", c.offset)
	}
	cancel()
	<-done
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for range 200 {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

func TestTelegram_TransportErrorsRedactToken(t *testing.T) {
	// http.Client.Do returns a *url.Error embedding the full request URL, and the
	// Telegram API puts the bot token in the URL path (/bot<token>/…). A transport
	// failure must NOT carry the token into the returned error (which could be
	// logged/journalled). Point the channel at an unreachable address to force the
	// transport error.
	const secret = "123456:ABC-SECRET-BOT-TOKEN-do-not-leak"
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	b := bus.New(j)
	t.Cleanup(b.Close)
	c := New(Config{
		Token:      secret,
		BaseURL:    "http://127.0.0.1:1", // port 1 → connection refused
		HTTPClient: &http.Client{Timeout: 2 * time.Second},
		Allowlist:  channel.NewAllowlist(nil),
		Bus:        b,
	})

	sendErr := c.Send(context.Background(), channel.Outbound{ChannelID: "123", Text: "hi"})
	if sendErr == nil {
		t.Fatal("expected a transport error sending to an unreachable host")
	}
	if strings.Contains(sendErr.Error(), secret) {
		t.Errorf("Send error leaked the bot token: %q", sendErr.Error())
	}
	if !strings.Contains(sendErr.Error(), "<redacted>") {
		t.Errorf("expected the token to be redacted in the error, got: %q", sendErr.Error())
	}
}

// TestGetUpdates_CapsOversizedResponse: a getUpdates response larger than
// tgAPIMaxResponseBytes is truncated by the io.LimitReader so the JSON decode
// fails (returns an error) instead of buffering an unbounded body — a buggy,
// compromised, or MITM'd Bot API endpoint can't OOM the daemon's long-poll loop.
func TestGetUpdates_CapsOversizedResponse(t *testing.T) {
	// A valid-prefix JSON whose total size exceeds the cap: an 8 MiB string field
	// makes the body run past tgAPIMaxResponseBytes, so the capped reader returns
	// a truncated (unterminated) JSON and Decode errors.
	big := strings.Repeat("A", tgAPIMaxResponseBytes)
	body := `{"ok":true,"result":[{"update_id":1,"message":{"message_id":1,"text":"` + big + `"}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	c := New(Config{Token: "T", BaseURL: srv.URL, HTTPClient: srv.Client()})
	if _, err := c.getUpdates(context.Background()); err == nil {
		t.Fatal("getUpdates accepted an over-cap response — the body is not size-bounded")
	}
}

// TestDispatchable_AdmitsPhotoAndCaption pins M476: the poll-loop gate must admit
// photo-only and caption-only messages, not just messages with Text. A photo's
// text rides in Caption and may be absent; gating on Text != "" dropped inbound
// images (M247) before handleInbound — which is the only place that fetches them.
func TestDispatchable_AdmitsPhotoAndCaption(t *testing.T) {
	cases := []struct {
		name string
		m    *tgMessage
		want bool
	}{
		{"nil", nil, false},
		{"empty", &tgMessage{}, false},
		{"text", &tgMessage{Text: "hi"}, true},
		{"caption only", &tgMessage{Caption: "a caption"}, true},
		{"photo only", &tgMessage{Photo: []tgPhotoSize{{FileID: "f"}}}, true},
	}
	for _, c := range cases {
		if got := dispatchable(c.m); got != c.want {
			t.Errorf("dispatchable(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}
