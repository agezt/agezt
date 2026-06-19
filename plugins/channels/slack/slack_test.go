// SPDX-License-Identifier: MIT

package slack

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/channel"
)

const secret = "shh-signing-secret"

func sign(ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + ts + ":"))
	mac.Write(body)
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

// postEvent sends body to the channel's events endpoint with a valid signature
// (unless badSig) and returns the recorder.
func postEvent(t *testing.T, c *Channel, body []byte, badSig bool, ts string) *httptest.ResponseRecorder {
	t.Helper()
	if ts == "" {
		ts = strconv.FormatInt(time.Now().Unix(), 10)
	}
	req := httptest.NewRequest(http.MethodPost, EventsPath, strings.NewReader(string(body)))
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	if badSig {
		req.Header.Set("X-Slack-Signature", "v0=deadbeef")
	} else {
		req.Header.Set("X-Slack-Signature", sign(ts, body))
	}
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, req)
	return rec
}

func TestSlack_URLVerification(t *testing.T) {
	c := New(Config{SigningSecret: secret})
	rec := postEvent(t, c, []byte(`{"type":"url_verification","challenge":"abc123"}`), false, "")
	if rec.Code != 200 {
		t.Fatalf("url_verification code = %d want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "abc123") {
		t.Errorf("challenge not echoed: %q", rec.Body.String())
	}
}

func TestSlack_BadSignatureRejected(t *testing.T) {
	c := New(Config{SigningSecret: secret})
	if rec := postEvent(t, c, []byte(`{"type":"url_verification","challenge":"x"}`), true, ""); rec.Code != 401 {
		t.Errorf("bad signature code = %d want 401", rec.Code)
	}
	// Stale timestamp (> 5 min) is rejected even with an otherwise-valid sig.
	old := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	if rec := postEvent(t, c, []byte(`{"type":"url_verification","challenge":"x"}`), false, old); rec.Code != 401 {
		t.Errorf("stale timestamp code = %d want 401", rec.Code)
	}
}

// slackAPI is an httptest stand-in for the Slack Web API: captures
// chat.postMessage bodies and returns {ok:true}.
func slackAPI(t *testing.T, posted chan<- map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat.postMessage") {
			http.NotFound(w, r)
			return
		}
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		posted <- m
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
}

func TestSlack_MessageDrivesAgentAndReplies(t *testing.T) {
	posted := make(chan map[string]any, 1)
	api := slackAPI(t, posted)
	defer api.Close()

	var got atomic.Value // the text the handler saw
	c := New(Config{
		SigningSecret: secret, Token: "xoxb-test", BaseURL: api.URL,
		HTTPClient: api.Client(),
		Allowlist:  channel.NewAllowlist([]string{"C1"}),
		Handler: func(_ context.Context, msg channel.UnifiedMessage, _ string) (channel.Reply, error) {
			got.Store(msg.Text)
			return channel.Reply{Text: "pong"}, nil
		},
	})

	body := []byte(`{"type":"event_callback","event":{"type":"message","channel":"C1","user":"U1","text":"ping","ts":"1700000000.000100"}}`)
	rec := postEvent(t, c, body, false, "")
	if rec.Code != 200 {
		t.Fatalf("event ACK code = %d want 200 (fast ack)", rec.Code)
	}

	select {
	case m := <-posted:
		if m["channel"] != "C1" || m["text"] != "pong" {
			t.Errorf("posted reply = %v want channel C1 / text pong", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for chat.postMessage reply")
	}
	if got.Load() != "ping" {
		t.Errorf("handler saw text %v want ping", got.Load())
	}
}

func TestSlack_ReplayDeduped(t *testing.T) {
	posted := make(chan map[string]any, 4)
	api := slackAPI(t, posted)
	defer api.Close()

	var calls int32
	c := New(Config{
		SigningSecret: secret, Token: "xoxb-test", BaseURL: api.URL, HTTPClient: api.Client(),
		Allowlist: channel.NewAllowlist([]string{"C1"}),
		Handler: func(_ context.Context, _ channel.UnifiedMessage, _ string) (channel.Reply, error) {
			atomic.AddInt32(&calls, 1)
			return channel.Reply{Text: "pong"}, nil
		},
	})

	// The same signed message (same ts) delivered twice — e.g. a replay of a
	// captured body — must drive the agent only once.
	body := []byte(`{"type":"event_callback","event":{"type":"message","channel":"C1","user":"U1","text":"ping","ts":"1700000000.000200"}}`)
	if rec := postEvent(t, c, body, false, ""); rec.Code != 200 {
		t.Fatalf("first delivery code = %d want 200", rec.Code)
	}
	select {
	case <-posted: // first reply delivered
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the first reply")
	}
	// Replay.
	if rec := postEvent(t, c, body, false, ""); rec.Code != 200 {
		t.Fatalf("replay code = %d want 200 (still ACKed)", rec.Code)
	}
	select {
	case m := <-posted:
		t.Errorf("replay should not drive a second run, but got a reply: %v", m)
	case <-time.After(300 * time.Millisecond):
		// good — no second reply
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("handler ran %d time(s); a replayed message must run exactly once", n)
	}
}

func TestSlack_IgnoresBotAndNonAllowlisted(t *testing.T) {
	posted := make(chan map[string]any, 4)
	api := slackAPI(t, posted)
	defer api.Close()

	var calls int32
	c := New(Config{
		SigningSecret: secret, Token: "xoxb", BaseURL: api.URL, HTTPClient: api.Client(),
		Allowlist: channel.NewAllowlist([]string{"C1"}),
		Handler: func(_ context.Context, _ channel.UnifiedMessage, _ string) (channel.Reply, error) {
			atomic.AddInt32(&calls, 1)
			return channel.Reply{Text: "reply"}, nil
		},
	})

	// A bot message (bot_id set) must be ignored entirely — no handler, no post.
	postEvent(t, c, []byte(`{"type":"event_callback","event":{"type":"message","channel":"C1","user":"U2","text":"hi","ts":"1.0","bot_id":"B9"}}`), false, "")
	// A non-allowlisted channel: handler not run, but a "not authorized" reply IS posted.
	postEvent(t, c, []byte(`{"type":"event_callback","event":{"type":"message","channel":"CX","user":"U3","text":"hi","ts":"1.0"}}`), false, "")

	// Give the async paths a moment.
	select {
	case m := <-posted:
		if m["channel"] != "CX" || !strings.Contains(m["text"].(string), "not authorized") {
			t.Errorf("expected a 'not authorized' post to CX, got %v", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected a not-authorized reply for the non-allowlisted channel")
	}
	time.Sleep(150 * time.Millisecond)
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Errorf("handler ran %d time(s); bot + non-allowlisted messages must not drive the agent", n)
	}
}
