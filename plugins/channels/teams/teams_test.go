// SPDX-License-Identifier: MIT

package teams

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

type hookMock struct {
	mu    sync.Mutex
	cards []map[string]any
	srv   *httptest.Server
}

func newHook(t *testing.T, status int) *hookMock {
	m := &hookMock{}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var card map[string]any
		_ = json.Unmarshal(b, &card)
		m.mu.Lock()
		m.cards = append(m.cards, card)
		m.mu.Unlock()
		w.WriteHeader(status)
		_, _ = io.WriteString(w, "1")
	}))
	t.Cleanup(m.srv.Close)
	return m
}
func (m *hookMock) count() int { m.mu.Lock(); defer m.mu.Unlock(); return len(m.cards) }

func newTestChannel(t *testing.T, hooks map[string]string) (*Channel, *journal.Journal) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	return New(Config{Webhooks: hooks, Bus: b}), j
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

// A message to a configured webhook name POSTs a MessageCard carrying the text and
// journals an outbound event.
func TestSend_PostsMessageCard(t *testing.T) {
	h := newHook(t, 200)
	c, j := newTestChannel(t, map[string]string{"general": h.srv.URL})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "general", Text: "deploy finished"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if h.count() != 1 {
		t.Fatalf("want 1 webhook POST, got %d", h.count())
	}
	h.mu.Lock()
	card := h.cards[0]
	h.mu.Unlock()
	if card["@type"] != "MessageCard" {
		t.Errorf("payload @type = %v, want MessageCard", card["@type"])
	}
	if card["text"] != "deploy finished" {
		t.Errorf("card text = %v, want the message", card["text"])
	}
	if countKind(t, j, event.KindChannelOutbound) != 1 {
		t.Error("expected one channel.outbound event")
	}
}

// An unknown webhook name is refused before any HTTP call (fail-closed).
func TestSend_UnknownNameRefused(t *testing.T) {
	h := newHook(t, 200)
	c, j := newTestChannel(t, map[string]string{"general": h.srv.URL})
	err := c.Send(context.Background(), channel.Outbound{ChannelID: "secret", Text: "hi"})
	if err == nil || !strings.Contains(err.Error(), "no webhook configured") {
		t.Fatalf("want unknown-name refusal, got %v", err)
	}
	if h.count() != 0 {
		t.Error("an unknown name must not POST")
	}
	if countKind(t, j, event.KindChannelOutbound) != 0 {
		t.Error("a refused message must not emit an outbound event")
	}
}

// Empty text is a no-op; a non-2xx webhook response surfaces as an error.
func TestSend_EmptyNoopAndErrorStatus(t *testing.T) {
	h := newHook(t, 200)
	c, _ := newTestChannel(t, map[string]string{"g": h.srv.URL})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "g", Text: "   "}); err != nil {
		t.Errorf("empty text should be a no-op, got %v", err)
	}
	if h.count() != 0 {
		t.Error("empty text must not POST")
	}

	bad := newHook(t, 400)
	c2, _ := newTestChannel(t, map[string]string{"g": bad.srv.URL})
	if err := c2.Send(context.Background(), channel.Outbound{ChannelID: "g", Text: "hi"}); err == nil {
		t.Error("a 400 from the webhook should surface as an error")
	}
}

// Names reports the configured webhook names (for Pulse fan-out + status).
func TestNames(t *testing.T) {
	c, _ := newTestChannel(t, map[string]string{"a": "http://x", "b": "http://y"})
	got := map[string]bool{}
	for _, n := range c.Names() {
		got[n] = true
	}
	if !got["a"] || !got["b"] || len(got) != 2 {
		t.Errorf("Names() = %v, want a,b", c.Names())
	}
}
