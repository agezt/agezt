// SPDX-License-Identifier: MIT

package openairesponses

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// modelsPayload is a trimmed copy of a real /models reply: two listed models out
// of priority order plus one hidden entry.
const modelsPayload = `{"models":[
 {"slug":"gpt-5.4","display_name":"GPT-5.4","visibility":"list","priority":16,"context_window":300000,"base_instructions":"instr-54"},
 {"slug":"codex-auto-review","display_name":"Codex Auto Review","visibility":"hide","priority":43,"base_instructions":"instr-review"},
 {"slug":"gpt-5.6-sol","display_name":"GPT-5.6-Sol","visibility":"list","priority":1,"context_window":400000,"max_context_window":500000,
  "input_modalities":["text","image"],"supported_in_api":true,"supports_parallel_tool_calls":true,"base_instructions":"instr-sol"}
]}`

// TestListModels checks the wire contract: the required client_version query
// param, the auth headers, and priority ordering of the reply.
func TestListModels(t *testing.T) {
	withLoopbackClient(t)
	var gotQuery, gotAuth, gotAccount, gotOriginator string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path = %q, want /models", r.URL.Path)
		}
		gotQuery = r.URL.Query().Get("client_version")
		gotAuth = r.Header.Get("Authorization")
		gotAccount = r.Header.Get("chatgpt-account-id")
		gotOriginator = r.Header.Get("originator")
		_, _ = w.Write([]byte(modelsPayload))
	}))
	defer srv.Close()

	models, err := ListModels(t.Context(), srv.URL, staticToken)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if gotQuery != ClientVersion {
		t.Errorf("client_version = %q, want %q (the backend 400s without it)", gotQuery, ClientVersion)
	}
	if gotAuth != "Bearer at-1" || gotAccount != "acc-1" || gotOriginator != originator {
		t.Errorf("headers = %q/%q/%q", gotAuth, gotAccount, gotOriginator)
	}
	want := []string{"gpt-5.6-sol", "gpt-5.4", "codex-auto-review"}
	if len(models) != len(want) {
		t.Fatalf("got %d models, want %d", len(models), len(want))
	}
	for i, slug := range want {
		if models[i].Slug != slug {
			t.Errorf("models[%d] = %q, want %q (priority order)", i, models[i].Slug, slug)
		}
	}
	sol := models[0]
	if !sol.Listed() || sol.MaxContextWindow != 500000 || sol.BaseInstructions != "instr-sol" {
		t.Errorf("sol entry decoded wrong: %+v", sol)
	}
	if len(sol.InputModalities) != 2 || !sol.ParallelTools || !sol.SupportedInAPI {
		t.Errorf("sol capability fields decoded wrong: %+v", sol)
	}
	if models[2].Listed() {
		t.Error("hidden model reported as listed")
	}
}

// TestListModels401Refreshes covers the reactive refresh-and-retry on 401.
func TestListModels401Refreshes(t *testing.T) {
	withLoopbackClient(t)
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") == "Bearer stale" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(modelsPayload))
	}))
	defer srv.Close()

	token := func(_ context.Context, force bool) (string, string, error) {
		if force {
			return "fresh", "acc-1", nil
		}
		return "stale", "acc-1", nil
	}
	models, err := ListModels(t.Context(), srv.URL, token)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if calls < 2 {
		t.Errorf("calls = %d, want >= 2 (401 then refreshed retry)", calls)
	}
	if len(models) != 3 {
		t.Errorf("got %d models after refresh, want 3", len(models))
	}
}

// TestListModelsErrors covers the no-token guard, a non-2xx status, and a reply
// carrying no usable models — each must be an error, never a silent empty set.
func TestListModelsErrors(t *testing.T) {
	if _, err := ListModels(t.Context(), "", nil); err == nil {
		t.Error("ListModels with no token source should fail")
	}

	withLoopbackClient(t)
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"bad status", `{"error":{"message":"nope"}}`, http.StatusBadRequest},
		{"no models", `{"models":[]}`, http.StatusOK},
		{"non-json", `not json`, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			if _, err := ListModels(t.Context(), srv.URL, staticToken); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// TestInstructionsPerModel is the fix for the stale-prompt half of the model
// drift: when discovery supplied a model's own base_instructions, the request
// must carry those, not the vendored Codex prompt. An unknown model still falls
// back, and a caller System prompt is appended in both cases.
func TestInstructionsPerModel(t *testing.T) {
	withLoopbackClient(t)
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"))
	}))
	defer srv.Close()

	p := New("chatgpt", "gpt-5.6-sol", staticToken)
	p.BaseURL = srv.URL
	p.Instructions = map[string]string{"gpt-5.6-sol": "SOL PROMPT"}

	req := agent.CompletionRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}}}
	if _, err := p.Complete(t.Context(), req); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got, _ := gotBody["instructions"].(string); got != "SOL PROMPT" {
		t.Errorf("instructions = %q, want the discovered per-model prompt", got)
	}

	// Unknown model → vendored fallback, with the caller's System appended.
	req.Model = "gpt-9-unknown"
	req.System = "BE TERSE"
	if _, err := p.Complete(t.Context(), req); err != nil {
		t.Fatalf("Complete unknown model: %v", err)
	}
	got, _ := gotBody["instructions"].(string)
	if !strings.HasPrefix(got, "You are Codex") || !strings.HasSuffix(got, "BE TERSE") {
		t.Errorf("fallback instructions = %.40q… (want vendored prompt + System)", got)
	}
}
