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
)

// haMock is a minimal Home Assistant REST stand-in: it records calls and serves
// canned state/service responses, asserting the bearer token on every request.
type haMock struct {
	mu       sync.Mutex
	srv      *httptest.Server
	gotAuth  string
	posted   map[string]json.RawMessage // "domain/service" → body
	getPaths []string
}

func newMock(t *testing.T) *haMock {
	t.Helper()
	m := &haMock{posted: map[string]json.RawMessage{}}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.gotAuth = r.Header.Get("Authorization")
		m.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/states":
			m.note(r.URL.Path)
			_, _ = io.WriteString(w, `[
				{"entity_id":"light.living_room","state":"on"},
				{"entity_id":"light.bedroom","state":"off"},
				{"entity_id":"lock.front_door","state":"locked"},
				{"entity_id":"sensor.temperature","state":"21.5"}
			]`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/states/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/states/")
			m.note(r.URL.Path)
			_, _ = io.WriteString(w, `{"entity_id":"`+id+`","state":"on"}`)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/services/"):
			ds := strings.TrimPrefix(r.URL.Path, "/api/services/")
			body, _ := io.ReadAll(r.Body)
			m.mu.Lock()
			m.posted[ds] = json.RawMessage(body)
			m.mu.Unlock()
			_, _ = io.WriteString(w, `[{"entity_id":"`+strings.ReplaceAll(ds, "/", ".")+`","state":"changed"}]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *haMock) note(p string) { m.mu.Lock(); m.getPaths = append(m.getPaths, p); m.mu.Unlock() }
func (m *haMock) postCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.posted)
}

func invoke(t *testing.T, tool *Tool, in map[string]any) (string, bool) {
	t.Helper()
	raw, _ := json.Marshal(in)
	res, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke returned a hard error: %v", err)
	}
	return res.Output, res.IsError
}

// A call_service to an allow-listed service POSTs to the right endpoint, merges
// entity_id into the data body, and carries the bearer token.
func TestCallService_AllowedPostsWithEntityAndToken(t *testing.T) {
	m := newMock(t)
	tool := &Tool{BaseURL: m.srv.URL, Token: "tok123", AllowedServices: []string{"light.turn_on"}}

	out, isErr := invoke(t, tool, map[string]any{
		"operation": "call_service",
		"domain":    "light",
		"service":   "turn_on",
		"entity_id": "light.living_room",
		"data":      map[string]any{"brightness": 128},
	})
	if isErr {
		t.Fatalf("allowed call should succeed, got error: %s", out)
	}
	if m.postCount() != 1 {
		t.Fatalf("want 1 POST, got %d", m.postCount())
	}
	body := m.posted["light/turn_on"]
	if body == nil {
		t.Fatalf("expected POST to /api/services/light/turn_on; got %v", m.posted)
	}
	var sent map[string]any
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("bad body: %v", err)
	}
	if sent["entity_id"] != "light.living_room" {
		t.Errorf("entity_id not merged into body: %v", sent)
	}
	if sent["brightness"].(float64) != 128 {
		t.Errorf("data not forwarded: %v", sent)
	}
	if m.gotAuth != "Bearer tok123" {
		t.Errorf("auth = %q, want Bearer tok123", m.gotAuth)
	}
}

// A service outside the allowlist is refused BEFORE any HTTP call (fail-closed),
// including a wildcard that doesn't cover it.
func TestCallService_NotAllowedRefused(t *testing.T) {
	m := newMock(t)
	// Allow only the light domain; a lock.unlock must be refused.
	tool := &Tool{BaseURL: m.srv.URL, Token: "t", AllowedServices: []string{"light.*"}}

	out, isErr := invoke(t, tool, map[string]any{
		"operation": "call_service", "domain": "lock", "service": "unlock", "entity_id": "lock.front_door",
	})
	if !isErr {
		t.Fatalf("lock.unlock should be refused, got ok: %s", out)
	}
	if !strings.Contains(out, "not in allowlist") {
		t.Errorf("want allowlist refusal, got %q", out)
	}
	if m.postCount() != 0 {
		t.Error("a refused service must not POST")
	}
}

// An empty service allowlist refuses every actuation (fail-closed default).
func TestCallService_EmptyAllowlistFailsClosed(t *testing.T) {
	m := newMock(t)
	tool := &Tool{BaseURL: m.srv.URL, Token: "t"} // no AllowedServices

	out, isErr := invoke(t, tool, map[string]any{
		"operation": "call_service", "domain": "light", "service": "turn_on", "entity_id": "light.x",
	})
	if !isErr || m.postCount() != 0 {
		t.Fatalf("empty allowlist must refuse, got isErr=%v posts=%d out=%s", isErr, m.postCount(), out)
	}
}

// The "domain.*" wildcard covers any service in that domain.
func TestCallService_DomainWildcard(t *testing.T) {
	m := newMock(t)
	tool := &Tool{BaseURL: m.srv.URL, Token: "t", AllowedServices: []string{"climate.*"}}
	_, isErr := invoke(t, tool, map[string]any{
		"operation": "call_service", "domain": "climate", "service": "set_temperature",
		"entity_id": "climate.bedroom", "data": map[string]any{"temperature": 20},
	})
	if isErr {
		t.Fatal("climate.* should permit climate.set_temperature")
	}
	if m.postCount() != 1 {
		t.Errorf("want 1 POST, got %d", m.postCount())
	}
}

// Reading a single allow-listed entity returns its state.
func TestGetStates_SingleAllowed(t *testing.T) {
	m := newMock(t)
	tool := &Tool{BaseURL: m.srv.URL, Token: "t", ReadEntities: []string{"light.living_room"}}
	out, isErr := invoke(t, tool, map[string]any{"operation": "get_states", "entity_id": "light.living_room"})
	if isErr {
		t.Fatalf("allowed read failed: %s", out)
	}
	if !strings.Contains(out, "light.living_room") {
		t.Errorf("expected the entity state, got %q", out)
	}
}

// Reading a non-allow-listed entity is refused without hitting the network.
func TestGetStates_SingleNotAllowed(t *testing.T) {
	m := newMock(t)
	tool := &Tool{BaseURL: m.srv.URL, Token: "t", ReadEntities: []string{"sensor.*"}}
	out, isErr := invoke(t, tool, map[string]any{"operation": "get_states", "entity_id": "lock.front_door"})
	if !isErr {
		t.Fatalf("non-allowed read should be refused, got: %s", out)
	}
	m.mu.Lock()
	n := len(m.getPaths)
	m.mu.Unlock()
	if n != 0 {
		t.Error("a refused read must not hit the network")
	}
}

// A bulk read is FILTERED to the allowlist: non-allowed entities never reach the
// model (anti-enumeration). sensor.* + light.living_room → exactly those.
func TestGetStates_BulkFilteredToAllowlist(t *testing.T) {
	m := newMock(t)
	tool := &Tool{BaseURL: m.srv.URL, Token: "t", ReadEntities: []string{"sensor.*", "light.living_room"}}
	out, isErr := invoke(t, tool, map[string]any{"operation": "get_states"})
	if isErr {
		t.Fatalf("bulk read failed: %s", out)
	}
	var got struct {
		Count  int              `json:"count"`
		States []map[string]any `json:"states"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("bad output JSON: %v\n%s", err, out)
	}
	if got.Count != 2 {
		t.Fatalf("want 2 kept (sensor.temperature + light.living_room), got %d: %s", got.Count, out)
	}
	for _, e := range got.States {
		id := e["entity_id"].(string)
		if id == "light.bedroom" || id == "lock.front_door" {
			t.Errorf("non-allowed entity leaked into bulk read: %s", id)
		}
	}
}

// An empty read allowlist refuses get_states entirely (fail-closed).
func TestGetStates_EmptyAllowlistFailsClosed(t *testing.T) {
	m := newMock(t)
	tool := &Tool{BaseURL: m.srv.URL, Token: "t"} // no ReadEntities
	out, isErr := invoke(t, tool, map[string]any{"operation": "get_states", "entity_id": "light.x"})
	if !isErr {
		t.Fatalf("empty read allowlist must refuse, got: %s", out)
	}
}

// Missing URL/token, unknown operation, and missing domain/service are clean
// tool errors (never a hard error).
func TestInvoke_InputValidation(t *testing.T) {
	m := newMock(t)
	full := &Tool{BaseURL: m.srv.URL, Token: "t", AllowedServices: []string{"*"}, ReadEntities: []string{"*"}}

	cases := []struct {
		name string
		tool *Tool
		in   map[string]any
		want string
	}{
		{"no url/token", &Tool{}, map[string]any{"operation": "get_states", "entity_id": "x.y"}, "base URL and token required"},
		{"empty op", full, map[string]any{"operation": ""}, "operation required"},
		{"unknown op", full, map[string]any{"operation": "frobnicate"}, "unknown operation"},
		{"no domain/service", full, map[string]any{"operation": "call_service", "entity_id": "light.x"}, "requires both domain and service"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, isErr := invoke(t, c.tool, c.in)
			if !isErr {
				t.Fatalf("want error result, got ok: %s", out)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("output %q does not contain %q", out, c.want)
			}
		})
	}
}

// The "*" wildcard permits any service / any entity (the allow-all escape hatch).
func TestMatchAllowed_StarMatchesAnything(t *testing.T) {
	if !matchAllowed([]string{"*"}, "lock.unlock") {
		t.Error(`"*" should match any service`)
	}
	if !matchAllowed([]string{"light.*"}, "light.toggle") {
		t.Error("light.* should match light.toggle")
	}
	if matchAllowed([]string{"light.*"}, "switch.toggle") {
		t.Error("light.* must NOT match switch.toggle")
	}
	if matchAllowed(nil, "light.turn_on") {
		t.Error("empty patterns must match nothing")
	}
	if matchAllowed([]string{"light.turn_on"}, "") {
		t.Error("empty target must not match")
	}
}

// Definition advertises only the enabled axes.
func TestDefinition_ReflectsEnabledAxes(t *testing.T) {
	readOnly := &Tool{ReadEntities: []string{"sensor.*"}}
	d := readOnly.Definition()
	if d.Name != "homeassistant" {
		t.Fatalf("name = %q", d.Name)
	}
	if !strings.Contains(d.Description, "get_states") || strings.Contains(d.Description, "call_service (") {
		t.Errorf("read-only tool should advertise get_states only: %q", d.Description)
	}
}
