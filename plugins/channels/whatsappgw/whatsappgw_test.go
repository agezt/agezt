// SPDX-License-Identifier: MIT

package whatsappgw

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

func TestParseWAHA(t *testing.T) {
	body := []byte(`{"event":"message","session":"default","payload":{"from":"12345@c.us","body":"hi there","id":"AAA","fromMe":false}}`)
	msgs := parseWAHA(body)
	if len(msgs) != 1 || msgs[0].from != "12345" || msgs[0].text != "hi there" || msgs[0].id != "AAA" {
		t.Fatalf("parseWAHA = %+v", msgs)
	}
	// fromMe is ignored (our own echoes).
	if got := parseWAHA([]byte(`{"event":"message","payload":{"from":"1@c.us","body":"x","fromMe":true}}`)); len(got) != 0 {
		t.Fatalf("fromMe should be ignored, got %+v", got)
	}
}

func TestParseEvolution(t *testing.T) {
	body := []byte(`{"event":"messages.upsert","data":{"key":{"remoteJid":"9876@s.whatsapp.net","id":"B1","fromMe":false},"message":{"conversation":"hello"}}}`)
	msgs := parseEvolution(body)
	if len(msgs) != 1 || msgs[0].from != "9876" || msgs[0].text != "hello" {
		t.Fatalf("parseEvolution = %+v", msgs)
	}
	// extendedTextMessage fallback.
	ext := []byte(`{"event":"messages.upsert","data":{"key":{"remoteJid":"5@s.whatsapp.net","fromMe":false},"message":{"extendedTextMessage":{"text":"quoted reply"}}}}`)
	if got := parseEvolution(ext); len(got) != 1 || got[0].text != "quoted reply" {
		t.Fatalf("extendedText = %+v", got)
	}
}

func TestSendWAHAShape(t *testing.T) {
	var path, body, apiKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		apiKey = r.Header.Get("X-Api-Key")
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}))
	defer srv.Close()
	ch := New(Config{Backend: BackendWAHA, BaseURL: srv.URL, Session: "s1", APIKey: "k", HTTPClient: srv.Client()})
	if err := ch.Send(context.Background(), channel.Outbound{ChannelID: "12345", Text: "yo"}); err != nil {
		t.Fatal(err)
	}
	if path != "/api/sendText" || apiKey != "k" {
		t.Fatalf("waha path=%q key=%q", path, apiKey)
	}
	var p map[string]any
	json.Unmarshal([]byte(body), &p)
	if p["chatId"] != "12345@c.us" || p["session"] != "s1" || p["text"] != "yo" {
		t.Fatalf("waha body = %q", body)
	}
}

func TestSendEvolutionShape(t *testing.T) {
	var path, body, apiKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		apiKey = r.Header.Get("apikey")
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}))
	defer srv.Close()
	ch := New(Config{Backend: BackendEvolution, BaseURL: srv.URL, Session: "inst", APIKey: "k", HTTPClient: srv.Client()})
	if err := ch.Send(context.Background(), channel.Outbound{ChannelID: "9876@s.whatsapp.net", Text: "hey"}); err != nil {
		t.Fatal(err)
	}
	if path != "/message/sendText/inst" || apiKey != "k" {
		t.Fatalf("evo path=%q key=%q", path, apiKey)
	}
	if !strings.Contains(body, `"number":"9876"`) || !strings.Contains(body, `"text":"hey"`) {
		t.Fatalf("evo body = %q (number should be bare)", body)
	}
}

func TestInboundDispatchAllowlist(t *testing.T) {
	var ran bool
	ch := New(Config{
		Backend:   BackendWAHA,
		BaseURL:   "http://unused",
		Allowlist: channel.NewAllowlist([]string{"12345"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (string, error) {
			ran = true
			if m.ChannelKind != "whatsappgw" || m.ChannelID != "12345" {
				t.Fatalf("msg = %+v", m)
			}
			return "", nil // no reply → no outbound attempt
		},
	})
	// Not allowlisted → handler must not run.
	ch.dispatch(context.Background(), inbound{from: "999", text: "hi", id: "x"})
	if ran {
		t.Fatal("handler ran for non-allowlisted sender")
	}
	// Allowlisted → handler runs.
	ch.dispatch(context.Background(), inbound{from: "12345", text: "ping", id: "y"})
	if !ran {
		t.Fatal("handler did not run for allowlisted sender")
	}
	// Dedup: same id is a no-op.
	ran = false
	ch.dispatch(context.Background(), inbound{from: "12345", text: "ping", id: "y"})
	if ran {
		t.Fatal("duplicate message id should be skipped")
	}
}
