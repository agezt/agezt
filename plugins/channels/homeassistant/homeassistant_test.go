// SPDX-License-Identifier: MIT

package homeassistant

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

type haMock struct {
	mu    sync.Mutex
	paths []string
	auth  []string
	msgs  []string
	srv   *httptest.Server
}

func newMock(t *testing.T, status int) *haMock {
	m := &haMock{}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		msg, _ := body["message"].(string)
		m.mu.Lock()
		m.paths = append(m.paths, r.URL.Path)
		m.auth = append(m.auth, r.Header.Get("Authorization"))
		m.msgs = append(m.msgs, msg)
		m.mu.Unlock()
		w.WriteHeader(status)
		_, _ = io.WriteString(w, "[]")
	}))
	t.Cleanup(m.srv.Close)
	return m
}
func (m *haMock) count() int { m.mu.Lock(); defer m.mu.Unlock(); return len(m.paths) }

func newTestChannel(t *testing.T, allow channel.Allowlist, base string) (*Channel, *journal.Journal) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	c := New(Config{BaseURL: base, Token: "llt-token", Allowlist: allow, Bus: b})
	return c, j
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

// A message to an allowlisted notify service POSTs to the right endpoint with the
// bearer token and the message body, and journals an outbound event.
func TestSend_PostsToNotifyService(t *testing.T) {
	m := newMock(t, 200)
	c, j := newTestChannel(t, channel.NewAllowlist([]string{"mobile_app_phone"}), m.srv.URL)
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "mobile_app_phone", Text: "dinner is ready"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if m.count() != 1 {
		t.Fatalf("want 1 notify POST, got %d", m.count())
	}
	m.mu.Lock()
	path, auth, msg := m.paths[0], m.auth[0], m.msgs[0]
	m.mu.Unlock()
	if path != "/api/services/notify/mobile_app_phone" {
		t.Errorf("path = %q, want the notify service endpoint", path)
	}
	if auth != "Bearer llt-token" {
		t.Errorf("auth = %q, want the bearer token", auth)
	}
	if msg != "dinner is ready" {
		t.Errorf("message = %q, want the text", msg)
	}
	if countKind(t, j, event.KindChannelOutbound) != 1 {
		t.Error("expected one channel.outbound event")
	}
}

// A non-allowlisted service is refused before any HTTP call (fail-closed).
func TestSend_NonAllowlistedRefused(t *testing.T) {
	m := newMock(t, 200)
	c, j := newTestChannel(t, channel.NewAllowlist([]string{"persistent_notification"}), m.srv.URL)
	err := c.Send(context.Background(), channel.Outbound{ChannelID: "tts", Text: "hi"})
	if err == nil || !strings.Contains(err.Error(), "not in allowlist") {
		t.Fatalf("want allowlist refusal, got %v", err)
	}
	if m.count() != 0 {
		t.Error("a refused service must not reach Home Assistant")
	}
	if countKind(t, j, event.KindChannelOutbound) != 0 {
		t.Error("a refused message must not emit an outbound event")
	}
}

// Empty text is a no-op; a non-2xx HA response surfaces as an error.
func TestSend_EmptyNoopAndErrorStatus(t *testing.T) {
	m := newMock(t, 200)
	c, _ := newTestChannel(t, channel.NewAllowlist([]string{"svc"}), m.srv.URL)
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "svc", Text: "  "}); err != nil {
		t.Errorf("empty text should be a no-op, got %v", err)
	}
	if m.count() != 0 {
		t.Error("empty text must not POST")
	}

	bad := newMock(t, 500)
	c2, _ := newTestChannel(t, channel.NewAllowlist([]string{"svc"}), bad.srv.URL)
	if err := c2.Send(context.Background(), channel.Outbound{ChannelID: "svc", Text: "hi"}); err == nil {
		t.Error("a 500 from Home Assistant should surface as an error")
	}
}

// No base URL / token configured errors rather than silently dropping.
func TestSend_UnconfiguredErrors(t *testing.T) {
	c, _ := newTestChannel(t, channel.NewAllowlist([]string{"svc"}), "")
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "svc", Text: "hi"}); err == nil {
		t.Error("send with no base URL should error")
	}
}
