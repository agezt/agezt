// SPDX-License-Identifier: MIT

package matrix

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

// fakeHomeserver records PUT sends and serves scripted whoami + /sync responses.
type fakeHomeserver struct {
	mu        sync.Mutex
	sent      []sentMsg  // captured outbound m.room.message bodies
	auth      []string   // Authorization header per request
	syncs     []syncResp // one per /sync call (last repeats)
	syncCalls int
	lastSince string // the `since` query param of the most recent /sync
	whoami    string // user_id to return
}

type sentMsg struct {
	room, txn, body string
}

func (f *fakeHomeserver) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		f.mu.Unlock()
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/account/whoami"):
			_ = json.NewEncoder(w).Encode(whoamiResp{UserID: f.whoami})
		case strings.HasSuffix(p, "/sync"):
			f.mu.Lock()
			f.lastSince = r.URL.Query().Get("since")
			var out syncResp
			if f.syncCalls < len(f.syncs) {
				out = f.syncs[f.syncCalls]
			} else if len(f.syncs) > 0 {
				out = f.syncs[len(f.syncs)-1]
			}
			f.syncCalls++
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(out)
		case strings.Contains(p, "/send/m.room.message/"):
			body, _ := io.ReadAll(r.Body)
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			// .../rooms/{room}/send/m.room.message/{txn}
			parts := strings.Split(p, "/")
			room, txn := "", parts[len(parts)-1]
			for i, seg := range parts {
				if seg == "rooms" && i+1 < len(parts) {
					room = parts[i+1]
				}
			}
			text, _ := m["body"].(string)
			f.mu.Lock()
			f.sent = append(f.sent, sentMsg{room: room, txn: txn, body: text})
			f.mu.Unlock()
			_, _ = io.WriteString(w, `{"event_id":"$evt"}`)
		default:
			w.WriteHeader(404)
		}
	}
}

func (f *fakeHomeserver) sentCount() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.sent) }

func newTestChannel(t *testing.T, srv *httptest.Server, allow channel.Allowlist, h channel.InboundHandler) (*Channel, *journal.Journal) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	c := New(Config{
		Homeserver: srv.URL,
		Token:      "syt_test_token",
		Allowlist:  allow,
		Bus:        b,
		Handler:    h,
		HTTPClient: srv.Client(),
	})
	c.userID = "@bot:test" // as resolveWhoami would set it
	return c, j
}

func textEvent(sender, body string) roomEvent {
	ev := roomEvent{Type: "m.room.message", Sender: sender, EventID: "$e1", TS: 1700000000000}
	ev.Content.MsgType = "m.text"
	ev.Content.Body = body
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

// An allowlisted room's text message drives the agent and the reply is PUT back
// to that room with the bot's bearer token.
func TestHandleInbound_AllowedRepliesViaSend(t *testing.T) {
	fh := &fakeHomeserver{whoami: "@bot:test"}
	srv := httptest.NewServer(fh.handler())
	defer srv.Close()

	var gotMsg channel.UnifiedMessage
	h := func(_ context.Context, m channel.UnifiedMessage, _ string) (channel.Reply, error) {
		gotMsg = m
		return channel.Reply{Text: "pong"}, nil
	}
	c, j := newTestChannel(t, srv, channel.NewAllowlist([]string{"!room:test"}), h)
	c.handleInbound(context.Background(), "!room:test", textEvent("@alice:test", "ping"))

	if gotMsg.Text != "ping" || gotMsg.ChannelID != "!room:test" || gotMsg.Sender != "@alice:test" {
		t.Errorf("handler got unexpected msg: %+v", gotMsg)
	}
	if fh.sentCount() != 1 {
		t.Fatalf("want 1 reply PUT, got %d", fh.sentCount())
	}
	if fh.sent[0].room != "!room:test" || fh.sent[0].body != "pong" {
		t.Errorf("reply = %+v, want room !room:test body pong", fh.sent[0])
	}
	if got := fh.auth[len(fh.auth)-1]; got != "Bearer syt_test_token" {
		t.Errorf("auth header = %q, want bearer token", got)
	}
	if countKind(t, j, event.KindChannelInbound) != 1 {
		t.Error("expected a channel.inbound event")
	}
	if countKind(t, j, event.KindChannelOutbound) != 1 {
		t.Error("expected a channel.outbound event")
	}
}

// A non-allowlisted room cannot drive the agent: the handler never runs and the
// sender is told once, fail-closed.
func TestHandleInbound_NotAllowedRefused(t *testing.T) {
	fh := &fakeHomeserver{whoami: "@bot:test"}
	srv := httptest.NewServer(fh.handler())
	defer srv.Close()

	ran := false
	h := func(_ context.Context, _ channel.UnifiedMessage, _ string) (channel.Reply, error) {
		ran = true
		return channel.Reply{Text: "should not run"}, nil
	}
	c, _ := newTestChannel(t, srv, channel.NewAllowlist([]string{"!allowed:test"}), h)
	c.handleInbound(context.Background(), "!stranger:test", textEvent("@mallory:test", "drive me"))

	if ran {
		t.Error("handler ran for a non-allowlisted room")
	}
	if fh.sentCount() != 1 || !strings.Contains(fh.sent[0].body, "not authorized") {
		t.Errorf("want one 'not authorized' notice, got %+v", fh.sent)
	}
}

// dispatchable gates the poll loop: only inbound text from a non-bot sender.
func TestDispatchable(t *testing.T) {
	c := &Channel{userID: "@bot:test"}
	ok := textEvent("@alice:test", "hi")
	if !c.dispatchable(ok) {
		t.Error("a valid inbound text event should dispatch")
	}
	self := textEvent("@bot:test", "my own reply")
	if c.dispatchable(self) {
		t.Error("the bot's OWN message must be skipped (else it loops)")
	}
	empty := textEvent("@alice:test", "   ")
	if c.dispatchable(empty) {
		t.Error("a whitespace-only body must be skipped")
	}
	notText := textEvent("@alice:test", "x")
	notText.Content.MsgType = "m.image"
	if c.dispatchable(notText) {
		t.Error("a non-text msgtype must be skipped (text-only scope)")
	}
	notMsg := textEvent("@alice:test", "x")
	notMsg.Type = "m.room.member"
	if c.dispatchable(notMsg) {
		t.Error("a non-message event type must be skipped")
	}
}

// sync parses the timeline, sends the bearer token, advances the since cursor,
// and forwards the cursor on the next request.
func TestSync_ParsesAndAdvancesCursor(t *testing.T) {
	fh := &fakeHomeserver{whoami: "@bot:test"}
	var b1 syncResp
	b1.NextBatch = "s-2"
	b1.Rooms.Join = map[string]struct {
		Timeline struct {
			Events []roomEvent `json:"events"`
		} `json:"timeline"`
	}{}
	room := struct {
		Timeline struct {
			Events []roomEvent `json:"events"`
		} `json:"timeline"`
	}{}
	room.Timeline.Events = []roomEvent{textEvent("@alice:test", "hello")}
	b1.Rooms.Join["!room:test"] = room
	fh.syncs = []syncResp{b1}
	srv := httptest.NewServer(fh.handler())
	defer srv.Close()
	c, _ := newTestChannel(t, srv, channel.NewAllowlist(nil), nil)
	c.since = "s-1"

	got, err := c.sync(context.Background())
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got.NextBatch != "s-2" {
		t.Errorf("next_batch = %q want s-2", got.NextBatch)
	}
	if fh.lastSince != "s-1" {
		t.Errorf("since param = %q, want s-1 (cursor forwarded)", fh.lastSince)
	}
	evs := got.Rooms.Join["!room:test"].Timeline.Events
	if len(evs) != 1 || evs[0].Content.Body != "hello" {
		t.Errorf("parsed timeline = %+v, want one 'hello'", evs)
	}
}

// send is a no-op for empty text and chunks a long body into multiple PUTs.
func TestSend_EmptyNoopAndChunks(t *testing.T) {
	fh := &fakeHomeserver{whoami: "@bot:test"}
	srv := httptest.NewServer(fh.handler())
	defer srv.Close()
	c, _ := newTestChannel(t, srv, channel.NewAllowlist(nil), nil)

	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "!r:test", Text: "   "}); err != nil {
		t.Fatalf("empty send: %v", err)
	}
	if fh.sentCount() != 0 {
		t.Errorf("empty text must not PUT; got %d", fh.sentCount())
	}

	long := strings.Repeat("a", matrixMaxChars+100) // forces a 2-chunk split
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "!r:test", Text: long}); err != nil {
		t.Fatalf("long send: %v", err)
	}
	if fh.sentCount() != 2 {
		t.Errorf("long text should split into 2 PUTs, got %d", fh.sentCount())
	}
	// Each chunk carries a distinct transaction id (idempotency).
	if fh.sent[0].txn == fh.sent[1].txn {
		t.Error("chunks must use distinct transaction ids")
	}
}

func TestResolveWhoami(t *testing.T) {
	fh := &fakeHomeserver{whoami: "@realbot:test"}
	srv := httptest.NewServer(fh.handler())
	defer srv.Close()
	c, _ := newTestChannel(t, srv, channel.NewAllowlist(nil), nil)
	c.userID = "" // clear the test default so resolveWhoami sets it
	if err := c.resolveWhoami(context.Background()); err != nil {
		t.Fatalf("whoami: %v", err)
	}
	if c.userID != "@realbot:test" {
		t.Errorf("userID = %q, want @realbot:test", c.userID)
	}
}
