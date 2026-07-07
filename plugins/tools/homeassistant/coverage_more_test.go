// SPDX-License-Identifier: MIT

package homeassistant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestHomeassistantCoverageDefinitionAndHelpers(t *testing.T) {
	// New: fail-closed defaults.
	tl := New()
	if tl == nil {
		t.Fatal("New should return a non-nil tool")
	}
	if tl.BaseURL != "" || tl.Token != "" {
		t.Fatal("New should leave BaseURL/Token empty")
	}
	if len(tl.AllowedServices) != 0 || len(tl.ReadEntities) != 0 {
		t.Fatal("New should leave both allowlists empty")
	}

	// client() returns DefaultTimeout client when HTTP is nil.
	if tl.client() == nil {
		t.Fatal("client() should return non-nil")
	}
	custom := &http.Client{}
	tl.HTTP = custom
	if tl.client() != custom {
		t.Fatal("client() should return the injected client")
	}
	tl.HTTP = nil

	// Definition: empty axes list "(nothing enabled)".
	def := tl.Definition()
	if def.Name != "homeassistant" {
		t.Fatalf("Name = %q", def.Name)
	}
	if def.Effect.Class != agent.EffectIrreversible {
		t.Fatalf("Effect.Class = %v, want %v", def.Effect.Class, agent.EffectIrreversible)
	}
	if !strings.Contains(def.Description, "(nothing enabled)") {
		t.Fatalf("description should list empty axes, got %q", def.Description)
	}
	schema := string(def.InputSchema)
	for _, want := range []string{`"get_states"`, `"call_service"`, `"entity_id"`, `"domain"`, `"service"`} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema should include %q, got %s", want, schema)
		}
	}

	// Definition with read + service axes listed in description.
	tl.ReadEntities = []string{"sensor.temp", "light.living_room"}
	tl.AllowedServices = []string{"light.turn_on", "light.turn_off"}
	def = tl.Definition()
	if !strings.Contains(def.Description, "get_states") || !strings.Contains(def.Description, "call_service") {
		t.Fatalf("description should list enabled axes, got %q", def.Description)
	}

	// Capabilities: empty → empty string (use a fresh tool to avoid prior
	// mutations).
	fresh := New()
	if got := fresh.Capabilities(); got != "" {
		t.Fatalf("empty axes capabilities = %q, want empty", got)
	}
	fresh.ReadEntities = []string{"a", "b"}
	fresh.AllowedServices = []string{"x"}
	if got := fresh.Capabilities(); got != "read=2, services=1" {
		t.Fatalf("populated capabilities = %q", got)
	}
}

func TestHomeassistantCoverageMatchAllowed(t *testing.T) {
	cases := []struct {
		patterns []string
		target   string
		want     bool
	}{
		{nil, "light.turn_on", false},
		{[]string{""}, "light.turn_on", false},
		{[]string{"  "}, "light.turn_on", false},
		{[]string{"*"}, "light.turn_on", true},
		{[]string{"light.turn_on"}, "light.turn_on", true},
		{[]string{"light.turn_on"}, "light.turn_off", false},
		{[]string{"light.*"}, "light.turn_off", true},
		{[]string{"light.*"}, "sensor.temp", false},
		{[]string{"LIGHT.*"}, "light.turn_on", true}, // case-insensitive
		{[]string{" climate.* "}, "climate.turn_on", true},
		{[]string{"sensor.*"}, "", false},
		{[]string{"sensor.*"}, "  ", false},
		// The wildcard check: "light.*" requires the prefix before the dot to
		// match the target's domain, not just end with ".*" plus a domain.
		{[]string{"a.*"}, "b.turn_on", false},
	}
	for _, tc := range cases {
		if got := matchAllowed(tc.patterns, tc.target); got != tc.want {
			t.Errorf("matchAllowed(%v, %q) = %v, want %v", tc.patterns, tc.target, got, tc.want)
		}
	}
}

func TestHomeassistantCoverageInvokeValidation(t *testing.T) {
	// No BaseURL/Token.
	tl := New()
	res, _ := tl.Invoke(context.Background(), json.RawMessage(`{"operation":"get_states"}`))
	if !res.IsError || !strings.Contains(res.Output, "base URL and token required") {
		t.Fatalf("no config = %+v", res)
	}

	// Parse error: soft error (the tool returns errResult, not a hard error).
	tl = &Tool{BaseURL: "http://ha", Token: "tok"}
	res, err := tl.Invoke(context.Background(), json.RawMessage(`{`))
	if err != nil {
		t.Fatalf("Invoke parse: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "invalid input") {
		t.Fatalf("parse error = %+v", res)
	}

	// Empty op.
	res, _ = tl.Invoke(context.Background(), json.RawMessage(`{}`))
	if !res.IsError || !strings.Contains(res.Output, "operation required") {
		t.Fatalf("empty op = %+v", res)
	}

	// Unknown op.
	res, _ = tl.Invoke(context.Background(), json.RawMessage(`{"operation":"wat"}`))
	if !res.IsError || !strings.Contains(res.Output, "unknown operation") {
		t.Fatalf("unknown op = %+v", res)
	}

	// get_states with no read allowlist.
	res, _ = tl.Invoke(context.Background(), json.RawMessage(`{"operation":"get_states"}`))
	if !res.IsError || !strings.Contains(res.Output, "not enabled") {
		t.Fatalf("no read allowlist = %+v", res)
	}

	// get_states with specific entity, not in allowlist.
	tl.ReadEntities = []string{"sensor.temp"}
	res, _ = tl.Invoke(context.Background(), json.RawMessage(`{"operation":"get_states","entity_id":"light.kitchen"}`))
	if !res.IsError || !strings.Contains(res.Output, "not in read allowlist") {
		t.Fatalf("entity not allowed = %+v", res)
	}

	// call_service with empty allowlist.
	res, _ = tl.Invoke(context.Background(), json.RawMessage(`{"operation":"call_service","domain":"light","service":"turn_on"}`))
	if !res.IsError || !strings.Contains(res.Output, "not in allowlist") {
		t.Fatalf("service not allowed = %+v", res)
	}

	// call_service missing domain or service.
	tl.AllowedServices = []string{"*"}
	res, _ = tl.Invoke(context.Background(), json.RawMessage(`{"operation":"call_service","service":"turn_on"}`))
	if !res.IsError || !strings.Contains(res.Output, "requires both") {
		t.Fatalf("call_service missing domain = %+v", res)
	}
}

func TestHomeassistantCoverageHTTPGetStates(t *testing.T) {
	tl := &Tool{
		BaseURL:         "",
		Token:           "tok",
		ReadEntities:    []string{"sensor.temp", "light.living_room"},
		AllowedServices: []string{"*"},
	}
	// Specific entity happy path.
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"entity_id":"sensor.temp","state":"21.5"}`))
	}))
	defer srv.Close()
	tl.BaseURL = srv.URL
	tl.HTTP = srv.Client()
	res, _ := tl.Invoke(context.Background(), json.RawMessage(`{"operation":"get_states","entity_id":"sensor.temp"}`))
	if res.IsError {
		t.Fatalf("get_states specific = %+v", res)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotPath != "/api/states/sensor.temp" {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(res.Output, "21.5") {
		t.Fatalf("output missing state: %s", res.Output)
	}

	// Bulk read filtered to allowlist.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
{"entity_id":"sensor.temp","state":"21"},
{"entity_id":"light.living_room","state":"on"},
{"entity_id":"camera.front_door","state":"recording"}
]`))
	}))
	defer srv2.Close()
	tl.BaseURL = srv2.URL
	tl.HTTP = srv2.Client()
	res, _ = tl.Invoke(context.Background(), json.RawMessage(`{"operation":"get_states"}`))
	if res.IsError {
		t.Fatalf("bulk get_states = %+v", res)
	}
	// camera.front_door should be filtered out; sensor and light should remain.
	if !strings.Contains(res.Output, "sensor.temp") || !strings.Contains(res.Output, "light.living_room") {
		t.Fatalf("bulk output missing allowed entities: %s", res.Output)
	}
	if strings.Contains(res.Output, "camera.front_door") {
		t.Fatalf("bulk output should NOT include filtered entity: %s", res.Output)
	}

	// Bulk read returns "too large" guidance on parse error.
	srv3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv3.Close()
	tl.BaseURL = srv3.URL
	tl.HTTP = srv3.Client()
	res, _ = tl.Invoke(context.Background(), json.RawMessage(`{"operation":"get_states"}`))
	if !res.IsError || !strings.Contains(res.Output, "could not parse") {
		t.Fatalf("parse error get_states = %+v", res)
	}

	// Non-2xx status.
	srv4 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv4.Close()
	tl.BaseURL = srv4.URL
	tl.HTTP = srv4.Client()
	res, _ = tl.Invoke(context.Background(), json.RawMessage(`{"operation":"get_states","entity_id":"sensor.temp"}`))
	if !res.IsError || !strings.Contains(res.Output, "status 404") {
		t.Fatalf("404 get_states = %+v", res)
	}
}

func TestHomeassistantCoverageHTTPCallService(t *testing.T) {
	tl := &Tool{
		Token:           "tok",
		AllowedServices: []string{"light.turn_on"},
	}
	var gotPath, gotMethod, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		_, _ = w.Write([]byte(`[{"entity_id":"light.living_room","state":"on"}]`))
	}))
	defer srv.Close()
	tl.BaseURL = srv.URL
	tl.HTTP = srv.Client()
	res, _ := tl.Invoke(context.Background(), json.RawMessage(`{"operation":"call_service","domain":"light","service":"turn_on","entity_id":"light.living_room","data":{"brightness":255}}`))
	if res.IsError {
		t.Fatalf("call_service = %+v", res)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/services/light/turn_on" {
		t.Fatalf("method/path = %s %s", gotMethod, gotPath)
	}
	if !strings.Contains(gotBody, `"entity_id"`) || !strings.Contains(gotBody, `"brightness"`) {
		t.Fatalf("body missing fields: %s", gotBody)
	}
	if !strings.Contains(res.Output, "called light.turn_on ok") {
		t.Fatalf("output missing ok: %s", res.Output)
	}

	// Invalid data JSON.
	res, _ = tl.Invoke(context.Background(), json.RawMessage(`{"operation":"call_service","domain":"light","service":"turn_on","data":"not-json"}`))
	if !res.IsError || !strings.Contains(res.Output, "invalid data") {
		t.Fatalf("invalid data = %+v", res)
	}

	// Non-2xx status.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv2.Close()
	tl.BaseURL = srv2.URL
	tl.HTTP = srv2.Client()
	res, _ = tl.Invoke(context.Background(), json.RawMessage(`{"operation":"call_service","domain":"light","service":"turn_on","entity_id":"x"}`))
	if !res.IsError || !strings.Contains(res.Output, "status 503") {
		t.Fatalf("503 call_service = %+v", res)
	}
}
