// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// haMockServer is a minimal Home Assistant REST stand-in for the CLI tests.
type haMockServer struct {
	mu       sync.Mutex
	srv      *httptest.Server
	lastPost map[string]json.RawMessage // "domain/service" → body
	auth     string
}

func newHAMock(t *testing.T) *haMockServer {
	t.Helper()
	m := &haMockServer{lastPost: map[string]json.RawMessage{}}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.auth = r.Header.Get("Authorization")
		m.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/states":
			_, _ = io.WriteString(w, `[
				{"entity_id":"light.living_room","state":"on"},
				{"entity_id":"sensor.temperature","state":"21.5"}
			]`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/states/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/states/")
			_, _ = io.WriteString(w, `{"entity_id":"`+id+`","state":"on"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/services":
			_, _ = io.WriteString(w, `[
				{"domain":"light","services":{"turn_on":{},"turn_off":{}}},
				{"domain":"lock","services":{"unlock":{}}}
			]`)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/services/"):
			ds := strings.TrimPrefix(r.URL.Path, "/api/services/")
			body, _ := io.ReadAll(r.Body)
			m.mu.Lock()
			m.lastPost[ds] = json.RawMessage(body)
			m.mu.Unlock()
			_, _ = io.WriteString(w, `[{"entity_id":"light.living_room","state":"off"}]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(m.srv.Close)
	return m
}

// configure points the CLI at the mock and sets a token via env.
func (m *haMockServer) configure(t *testing.T) {
	t.Helper()
	t.Setenv("AGEZT_HOMEASSISTANT_URL", m.srv.URL)
	t.Setenv("AGEZT_HOMEASSISTANT_TOKEN", "tok123")
}

func runHA(args ...string) (string, string, int) {
	var out, errOut bytes.Buffer
	code := cmdHA(args, &out, &errOut)
	return out.String(), errOut.String(), code
}

func TestHA_NoConfigFailsClosed(t *testing.T) {
	t.Setenv("AGEZT_HOMEASSISTANT_URL", "")
	t.Setenv("AGEZT_HOMEASSISTANT_TOKEN", "")
	_, errOut, code := runHA("states")
	if code != 2 || !strings.Contains(errOut, "HOMEASSISTANT_URL") {
		t.Fatalf("want exit 2 + hint, got code=%d err=%q", code, errOut)
	}
}

func TestHA_Help(t *testing.T) {
	out, _, code := runHA("--help")
	if code != 0 || !strings.Contains(out, "states") || !strings.Contains(out, "call <domain.service>") {
		t.Fatalf("help missing commands; code=%d out=%q", code, out)
	}
}

func TestHA_StatesAll(t *testing.T) {
	m := newHAMock(t)
	m.configure(t)
	out, _, code := runHA("states")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(out, "light.living_room = on") || !strings.Contains(out, "sensor.temperature = 21.5") {
		t.Errorf("states listing wrong:\n%s", out)
	}
	if !strings.Contains(out, "(2 entities)") {
		t.Errorf("missing count: %s", out)
	}
	if m.auth != "Bearer tok123" {
		t.Errorf("auth = %q", m.auth)
	}
}

func TestHA_StatesOneEntityJSON(t *testing.T) {
	m := newHAMock(t)
	m.configure(t)
	out, _, code := runHA("states", "light.living_room")
	if code != 0 || !strings.Contains(out, `"entity_id": "light.living_room"`) {
		t.Fatalf("pretty single-entity state expected; code=%d out=%q", code, out)
	}
}

func TestHA_ServicesListsDomainDotService(t *testing.T) {
	m := newHAMock(t)
	m.configure(t)
	out, _, code := runHA("services")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	for _, want := range []string{"light.turn_on", "light.turn_off", "lock.unlock", "(3 services)"} {
		if !strings.Contains(out, want) {
			t.Errorf("services output missing %q:\n%s", want, out)
		}
	}
}

func TestHA_CallPostsServiceWithEntityAndData(t *testing.T) {
	m := newHAMock(t)
	m.configure(t)
	out, _, code := runHA("call", "light.turn_on", "--entity", "light.living_room", "--data", `{"brightness":128}`)
	if code != 0 {
		t.Fatalf("exit=%d out=%q", code, out)
	}
	if !strings.Contains(out, "called light.turn_on ok") {
		t.Errorf("missing confirmation: %s", out)
	}
	m.mu.Lock()
	body := m.lastPost["light/turn_on"]
	m.mu.Unlock()
	if body == nil {
		t.Fatalf("no POST to light/turn_on; got %v", m.lastPost)
	}
	var sent map[string]any
	_ = json.Unmarshal(body, &sent)
	if sent["entity_id"] != "light.living_room" {
		t.Errorf("entity_id not merged: %v", sent)
	}
	if sent["brightness"].(float64) != 128 {
		t.Errorf("data not forwarded: %v", sent)
	}
}

func TestHA_CallRequiresDottedTarget(t *testing.T) {
	m := newHAMock(t)
	m.configure(t)
	_, errOut, code := runHA("call", "turn_on") // no domain.service
	if code != 2 || !strings.Contains(errOut, "domain.service") {
		t.Fatalf("want exit 2 + usage, got code=%d err=%q", code, errOut)
	}
}

func TestHA_CallBadDataJSON(t *testing.T) {
	m := newHAMock(t)
	m.configure(t)
	_, errOut, code := runHA("call", "light.turn_on", "--data", "{not json}")
	if code != 2 || !strings.Contains(errOut, "valid JSON") {
		t.Fatalf("want exit 2 on bad --data, got code=%d err=%q", code, errOut)
	}
}

func TestHA_UnknownSubcommand(t *testing.T) {
	m := newHAMock(t)
	m.configure(t)
	_, errOut, code := runHA("frobnicate")
	if code != 2 || !strings.Contains(errOut, "unknown command") {
		t.Fatalf("want exit 2, got code=%d err=%q", code, errOut)
	}
}

func TestHA_Non2xxSurfacesError(t *testing.T) {
	// A server that always 500s → the CLI reports HTTP 500, exit 1.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, "boom")
	}))
	t.Cleanup(srv.Close)
	t.Setenv("AGEZT_HOMEASSISTANT_URL", srv.URL)
	t.Setenv("AGEZT_HOMEASSISTANT_TOKEN", "t")
	_, errOut, code := runHA("states")
	if code != 1 || !strings.Contains(errOut, "HTTP 500") {
		t.Fatalf("want exit 1 + HTTP 500, got code=%d err=%q", code, errOut)
	}
}
