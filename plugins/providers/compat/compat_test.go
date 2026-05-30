// SPDX-License-Identifier: MIT

package compat_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/plugins/providers/compat"
)

// genTestVertexSA generates an RSA keypair and embeds it in a valid
// service-account JSON. tokenURI is wired so the resulting JWT-bearer
// exchange hits an httptest server instead of the real Google token
// endpoint.
func genTestVertexSA(t *testing.T, tokenURI string) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen rsa key: %v", err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	raw, err := json.Marshal(map[string]any{
		"type":           "service_account",
		"project_id":     "test-project",
		"private_key":    string(pemBytes),
		"private_key_id": "test-kid",
		"client_email":   "test@test-project.iam.gserviceaccount.com",
		"token_uri":      tokenURI,
	})
	if err != nil {
		t.Fatalf("marshal sa json: %v", err)
	}
	return raw
}

// writeTempFile writes content to a t.TempDir() file and returns its path.
func writeTempFile(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

// ---- fixtures ----

func newAnthCatalogEntry(api string) *catalog.Provider {
	return &catalog.Provider{
		ID: "anthropic", Name: "Anthropic",
		Env: []string{"ANTHROPIC_API_KEY"},
		NPM: "@ai-sdk/anthropic", API: api,
		Models: map[string]*catalog.Model{
			"claude-opus-4-7": {
				ID: "claude-opus-4-7", Name: "Claude Opus 4.7",
				Cost: &catalog.Cost{Input: 5, Output: 25},
			},
		},
	}
}

func newOllamaCatalogEntry(api string) *catalog.Provider {
	return &catalog.Provider{
		ID: "ollama-local", Name: "Ollama (local)",
		NPM: "@ai-sdk/ollama", API: api,
		Models: map[string]*catalog.Model{
			"llama3.2": {ID: "llama3.2", Name: "llama3.2"},
		},
	}
}

func newOpenAICatalogEntry(api string) *catalog.Provider {
	return &catalog.Provider{
		ID: "openai", Name: "OpenAI",
		Env: []string{"OPENAI_API_KEY"},
		NPM: "@ai-sdk/openai", API: api,
		Models: map[string]*catalog.Model{
			"gpt-4o-mini": {ID: "gpt-4o-mini"},
		},
	}
}

// ---- Build dispatches to the right adapter ----

func TestBuild_AnthropicFamilyRoutesToAnthropicWire(t *testing.T) {
	var seen struct {
		method, url, apiKey, anthVer string
		body                         map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.method = r.Method
		seen.url = r.URL.Path
		seen.apiKey = r.Header.Get("x-api-key")
		seen.anthVer = r.Header.Get("anthropic-version")
		_ = json.NewDecoder(r.Body).Decode(&seen.body)
		// Minimal Anthropic-shaped success.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_test", "type": "message", "role": "assistant",
			"model":       "claude-opus-4-7",
			"stop_reason": "end_turn",
			"content":     []map[string]any{{"type": "text", "text": "hi from anthropic-compat"}},
			"usage":       map[string]any{"input_tokens": 3, "output_tokens": 5},
		})
	}))
	defer srv.Close()

	// models.dev's anthropic `api` includes the version segment; the
	// adapter appends only "/messages". Mirror that here.
	entry := newAnthCatalogEntry(srv.URL + "/v1")
	prov, model, err := compat.Build(entry, "claude-opus-4-7", func(string) string { return "test-key" })
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if model != "claude-opus-4-7" {
		t.Errorf("model=%q", model)
	}
	if prov.Name() != "anthropic" {
		t.Errorf("Name()=%q want catalog id 'anthropic'", prov.Name())
	}
	resp, err := prov.Complete(context.Background(), agent.CompletionRequest{
		Model:    "claude-opus-4-7",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hi from anthropic-compat" {
		t.Errorf("response: %q", resp.Message.Content)
	}
	if seen.method != "POST" {
		t.Errorf("method=%q", seen.method)
	}
	if seen.url != "/v1/messages" {
		t.Errorf("url path=%q want /v1/messages", seen.url)
	}
	if seen.apiKey != "test-key" {
		t.Errorf("x-api-key=%q want test-key", seen.apiKey)
	}
	if seen.anthVer == "" {
		t.Error("anthropic-version header missing")
	}
	if seen.body["model"] != "claude-opus-4-7" {
		t.Errorf("body.model=%v", seen.body["model"])
	}
}

// TestBuild_AnthropicThirdPartyBaseURLNoDoubleVersion is a regression test for
// third-party Anthropic-shaped providers (MiniMax coding-plan, Kimi-for-coding)
// whose models.dev `api` already carries a versioned path like
// "…/anthropic/v1". The adapter must append only "/messages" — NOT
// "/v1/messages" — so the request lands on "/anthropic/v1/messages", not the
// doubled "/anthropic/v1/v1/messages" that 404s.
func TestBuild_AnthropicThirdPartyBaseURLNoDoubleVersion(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_test", "type": "message", "role": "assistant",
			"model": "MiniMax-M2.7", "stop_reason": "end_turn",
			"content": []map[string]any{{"type": "text", "text": "ok"}},
			"usage":   map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer srv.Close()

	// Mirror the models.dev minimax-coding-plan entry: anthropic family, an
	// `api` that already ends in a versioned segment.
	entry := &catalog.Provider{
		ID: "minimax-coding-plan", Name: "MiniMax (coding plan)",
		Env: []string{"MINIMAX_API_KEY"}, NPM: "@ai-sdk/anthropic",
		API:    srv.URL + "/anthropic/v1",
		Models: map[string]*catalog.Model{"MiniMax-M2.7": {ID: "MiniMax-M2.7"}},
	}
	prov, _, err := compat.Build(entry, "MiniMax-M2.7", func(string) string { return "k" })
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := prov.Complete(context.Background(), agent.CompletionRequest{
		Model:    "MiniMax-M2.7",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "ping"}},
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if gotPath != "/anthropic/v1/messages" {
		t.Errorf("path = %q want /anthropic/v1/messages (no doubled version segment)", gotPath)
	}
}

func TestBuild_OpenAIFamilyRoutesToOpenAIWire(t *testing.T) {
	var seen struct {
		auth, path string
		body       map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.auth = r.Header.Get("Authorization")
		seen.path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&seen.body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "cmpl-x", "object": "chat.completion", "model": "gpt-4o-mini",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "hi from openai-compat"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 4},
		})
	}))
	defer srv.Close()

	entry := newOpenAICatalogEntry(srv.URL + "/v1")
	prov, model, err := compat.Build(entry, "gpt-4o-mini", func(string) string { return "sk-key" })
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if model != "gpt-4o-mini" {
		t.Errorf("model=%q", model)
	}
	if prov.Name() != "openai" {
		t.Errorf("Name()=%q want catalog id 'openai'", prov.Name())
	}
	resp, err := prov.Complete(context.Background(), agent.CompletionRequest{
		Model:    "gpt-4o-mini",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hi from openai-compat" {
		t.Errorf("response: %q", resp.Message.Content)
	}
	if seen.path != "/v1/chat/completions" {
		t.Errorf("url path=%q want /v1/chat/completions", seen.path)
	}
	if seen.auth != "Bearer sk-key" {
		t.Errorf("auth=%q", seen.auth)
	}
	if seen.body["model"] != "gpt-4o-mini" {
		t.Errorf("body.model=%v", seen.body["model"])
	}
}

func TestBuild_OpenAICompatibleFamilyRoutesToOpenAIWire(t *testing.T) {
	// "Groq-shaped" entry — openai-compatible npm tag, /openai/v1 base.
	var hit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "llama-3.3-70b-versatile",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "ok"},
				"finish_reason": "stop",
			}},
		})
	}))
	defer srv.Close()

	entry := &catalog.Provider{
		ID: "groq", Name: "Groq",
		Env: []string{"GROQ_API_KEY"},
		NPM: "@ai-sdk/openai-compatible",
		API: srv.URL + "/openai/v1",
		Models: map[string]*catalog.Model{
			"llama-3.3-70b-versatile": {ID: "llama-3.3-70b-versatile"},
		},
	}
	prov, _, err := compat.Build(entry, "llama-3.3-70b-versatile",
		func(name string) string {
			if name == "GROQ_API_KEY" {
				return "gsk-test"
			}
			return ""
		})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if prov.Name() != "groq" {
		t.Errorf("Name()=%q want 'groq'", prov.Name())
	}
	if _, err := prov.Complete(context.Background(), agent.CompletionRequest{
		Model:    "llama-3.3-70b-versatile",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "ping"}},
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if hit != "/openai/v1/chat/completions" {
		t.Errorf("hit=%q want /openai/v1/chat/completions", hit)
	}
}

func TestBuild_OllamaFamilyRoutesToOllamaWire(t *testing.T) {
	var seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "llama3.2",
			"message": map[string]any{
				"role":    "assistant",
				"content": "ok from ollama-compat",
			},
			"done":              true,
			"prompt_eval_count": 4,
			"eval_count":        2,
		})
	}))
	defer srv.Close()

	entry := newOllamaCatalogEntry(srv.URL)
	prov, _, err := compat.Build(entry, "llama3.2", nil) // local: no creds needed
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if prov.Name() != "ollama-local" {
		t.Errorf("Name()=%q want 'ollama-local'", prov.Name())
	}
	resp, err := prov.Complete(context.Background(), agent.CompletionRequest{
		Model:    "llama3.2",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !strings.Contains(resp.Message.Content, "ollama-compat") {
		t.Errorf("response: %q", resp.Message.Content)
	}
	if seenPath != "/api/chat" {
		t.Errorf("url path=%q want /api/chat", seenPath)
	}
}

// ---- Build rejects bad inputs ----

func TestBuild_UnsupportedFamilyReturnsErr(t *testing.T) {
	// M1.n wired every family in models.dev. The default branch is
	// now load-bearing only as a safety net for *future* npm tags
	// the catalog might surface. Synthesise an entry whose Family()
	// resolves to FamilyUnknown by using a made-up npm name.
	entry := &catalog.Provider{
		ID: "future-vendor", Name: "Future Vendor",
		Env: []string{"FV_API_KEY"},
		NPM: "@future-vendor/some-sdk", API: "https://api.example.invalid",
		Models: map[string]*catalog.Model{"x": {ID: "x"}},
	}
	_, _, err := compat.Build(entry, "x", func(string) string { return "k" })
	if !errors.Is(err, compat.ErrFamilyUnsupported) {
		t.Errorf("got %v want ErrFamilyUnsupported", err)
	}
}

func TestBuild_VertexFamilyRoutesToVertexWire(t *testing.T) {
	// Two test servers: OAuth token endpoint + Vertex generateContent.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "ya29.test", "expires_in": 3600, "token_type": "Bearer",
		})
	}))
	defer tokenSrv.Close()

	var seen struct{ path, authz string }
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.path = r.URL.Path
		seen.authz = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content":      map[string]any{"role": "model", "parts": []map[string]any{{"text": "hi from vertex"}}},
				"finishReason": "STOP",
			}},
		})
	}))
	defer apiSrv.Close()

	// Generate a real service-account JSON with an RSA keypair so
	// the JWT signing actually works.
	saJSON := genTestVertexSA(t, tokenSrv.URL)
	saPath := writeTempFile(t, saJSON)

	entry := &catalog.Provider{
		ID: "google-vertex", Name: "Google Vertex",
		Env: []string{"GOOGLE_VERTEX_PROJECT", "GOOGLE_VERTEX_LOCATION", "GOOGLE_APPLICATION_CREDENTIALS"},
		NPM: "@ai-sdk/google-vertex",
		API: apiSrv.URL, // operator override so the URL builder hits our test server
		Models: map[string]*catalog.Model{
			"gemini-1.5-flash": {ID: "gemini-1.5-flash"},
		},
	}
	prov, _, err := compat.Build(entry, "gemini-1.5-flash",
		func(name string) string {
			switch name {
			case "GOOGLE_APPLICATION_CREDENTIALS":
				return saPath
			case "GOOGLE_VERTEX_LOCATION":
				return "us-central1"
			case "GOOGLE_VERTEX_PROJECT":
				return "my-proj"
			}
			return ""
		})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if prov.Name() != "google-vertex" {
		t.Errorf("Name()=%q want 'google-vertex'", prov.Name())
	}
	resp, err := prov.Complete(context.Background(), agent.CompletionRequest{
		Model:    "gemini-1.5-flash",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hi from vertex" {
		t.Errorf("content=%q", resp.Message.Content)
	}
	wantPath := "/v1/projects/my-proj/locations/us-central1/publishers/google/models/gemini-1.5-flash:generateContent"
	if seen.path != wantPath {
		t.Errorf("path=%q want %q", seen.path, wantPath)
	}
	if seen.authz != "Bearer ya29.test" {
		t.Errorf("authz=%q", seen.authz)
	}
}

func TestBuild_VertexMissingCredsRefused(t *testing.T) {
	entry := &catalog.Provider{
		ID: "google-vertex", NPM: "@ai-sdk/google-vertex",
		Env:    []string{"GOOGLE_VERTEX_PROJECT", "GOOGLE_VERTEX_LOCATION", "GOOGLE_APPLICATION_CREDENTIALS"},
		Models: map[string]*catalog.Model{"gemini-1.5-flash": {ID: "gemini-1.5-flash"}},
	}
	// Only project set, no creds file path, no location.
	_, _, err := compat.Build(entry, "gemini-1.5-flash", func(name string) string {
		if name == "GOOGLE_VERTEX_PROJECT" {
			return "p"
		}
		return ""
	})
	if !errors.Is(err, compat.ErrMissingCredentials) {
		t.Errorf("got %v want ErrMissingCredentials", err)
	}
	if err == nil || !strings.Contains(err.Error(), "GOOGLE_APPLICATION_CREDENTIALS") {
		t.Errorf("error should mention GOOGLE_APPLICATION_CREDENTIALS; got %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "M1.n.x") {
		t.Errorf("error should name the ADC/workload-identity deferral; got %v", err)
	}
}

func TestBuild_BedrockFamilyRoutesToBedrockWire(t *testing.T) {
	var seen struct {
		path, auth string
		body       map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.path = r.URL.Path
		seen.auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&seen.body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_b", "type": "message", "role": "assistant",
			"content":     []map[string]any{{"type": "text", "text": "hi from bedrock"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 2},
		})
	}))
	defer srv.Close()

	entry := &catalog.Provider{
		ID: "amazon-bedrock", Name: "Amazon Bedrock",
		Env: []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION", "AWS_BEARER_TOKEN_BEDROCK"},
		NPM: "@ai-sdk/amazon-bedrock", API: srv.URL, // override host so we don't hit AWS
		Models: map[string]*catalog.Model{
			"anthropic.claude-opus-4-7": {ID: "anthropic.claude-opus-4-7"},
		},
	}
	prov, _, err := compat.Build(entry, "anthropic.claude-opus-4-7",
		func(name string) string {
			switch name {
			case "AWS_BEARER_TOKEN_BEDROCK":
				return "br-test-token"
			case "AWS_REGION":
				return "us-east-1"
			}
			return ""
		})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if prov.Name() != "amazon-bedrock" {
		t.Errorf("Name()=%q want 'amazon-bedrock'", prov.Name())
	}
	resp, err := prov.Complete(context.Background(), agent.CompletionRequest{
		Model:    "anthropic.claude-opus-4-7",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hi from bedrock" {
		t.Errorf("response: %q", resp.Message.Content)
	}
	if seen.path != "/model/anthropic.claude-opus-4-7/invoke" {
		t.Errorf("path=%q want /model/anthropic.claude-opus-4-7/invoke", seen.path)
	}
	if seen.auth != "Bearer br-test-token" {
		t.Errorf("auth=%q", seen.auth)
	}
	if seen.body["anthropic_version"] != "bedrock-2023-05-31" {
		t.Errorf("anthropic_version=%v", seen.body["anthropic_version"])
	}
	if _, ok := seen.body["model"]; ok {
		t.Errorf("body should NOT carry `model` (it's in the URL)")
	}
}

func TestBuild_BedrockMissingBearerTokenRefused(t *testing.T) {
	entry := &catalog.Provider{
		ID: "amazon-bedrock", NPM: "@ai-sdk/amazon-bedrock",
		Env:    []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION", "AWS_BEARER_TOKEN_BEDROCK"},
		Models: map[string]*catalog.Model{"anthropic.claude-opus-4-7": {ID: "anthropic.claude-opus-4-7"}},
	}
	// Only AWS_REGION set; no bearer token.
	_, _, err := compat.Build(entry, "anthropic.claude-opus-4-7", func(name string) string {
		if name == "AWS_REGION" {
			return "us-east-1"
		}
		return ""
	})
	if !errors.Is(err, compat.ErrMissingCredentials) {
		t.Errorf("got %v want ErrMissingCredentials", err)
	}
	// Post-M1.m.x: error message lists BOTH auth paths so the
	// operator sees both options. Previously named the SigV4
	// deferral; now there's no deferral to name.
	if err == nil || !strings.Contains(err.Error(), "AWS_BEARER_TOKEN_BEDROCK") || !strings.Contains(err.Error(), "AWS_ACCESS_KEY_ID") {
		t.Errorf("error should list both auth paths (bearer + SigV4); got %v", err)
	}
}

func TestBuild_BedrockMissingRegionRefusedUnlessAPIOverride(t *testing.T) {
	entry := &catalog.Provider{
		ID: "amazon-bedrock", NPM: "@ai-sdk/amazon-bedrock",
		Env: []string{"AWS_BEARER_TOKEN_BEDROCK", "AWS_REGION"}, API: "",
		Models: map[string]*catalog.Model{"anthropic.claude-opus-4-7": {ID: "anthropic.claude-opus-4-7"}},
	}
	// Bearer set, no region, no api override.
	_, _, err := compat.Build(entry, "anthropic.claude-opus-4-7", func(name string) string {
		if name == "AWS_BEARER_TOKEN_BEDROCK" {
			return "tok"
		}
		return ""
	})
	if !errors.Is(err, compat.ErrMissingCredentials) {
		t.Errorf("got %v want ErrMissingCredentials", err)
	}
	// Post-M1.m.x: the message now explains the region requirement
	// in terms of the credential scope (SigV4) and the host URL
	// (bearer) rather than naming custom.json directly.
	if err == nil || !strings.Contains(err.Error(), "AWS_REGION") {
		t.Errorf("error should mention AWS_REGION requirement; got %v", err)
	}
}

func TestBuild_AzureFamilyRoutesToOpenAIWireWithAzureURL(t *testing.T) {
	// Azure: openai body, api-key auth, resource+deployment+api-version URL.
	var seen struct {
		path, query, authz, apiKey string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.path = r.URL.Path
		seen.query = r.URL.RawQuery
		seen.authz = r.Header.Get("Authorization")
		seen.apiKey = r.Header.Get("api-key")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "hi from azure"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 2},
		})
	}))
	defer srv.Close()

	// Catalog entry with an explicit `api` URL so we don't need a
	// resource name to build the host. This is the operator-override
	// path; the resource-only path is exercised separately.
	entry := &catalog.Provider{
		ID: "azure", Name: "Azure",
		Env: []string{"AZURE_RESOURCE_NAME", "AZURE_API_KEY"},
		NPM: "@ai-sdk/azure",
		API: srv.URL, // operator override
		Models: map[string]*catalog.Model{
			"my-gpt4o-deployment": {ID: "my-gpt4o-deployment"},
		},
	}
	prov, _, err := compat.Build(entry, "my-gpt4o-deployment",
		func(name string) string {
			switch name {
			case "AZURE_API_KEY":
				return "az-secret"
			}
			return ""
		})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if prov.Name() != "azure" {
		t.Errorf("Name()=%q want 'azure'", prov.Name())
	}
	resp, err := prov.Complete(context.Background(), agent.CompletionRequest{
		Model:    "my-gpt4o-deployment",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hi from azure" {
		t.Errorf("response: %q", resp.Message.Content)
	}
	if seen.path != "/openai/deployments/my-gpt4o-deployment/chat/completions" {
		t.Errorf("path=%q", seen.path)
	}
	if !strings.Contains(seen.query, "api-version=") {
		t.Errorf("query missing api-version: %q", seen.query)
	}
	if seen.authz != "" {
		t.Errorf("Authorization should be absent for azure; got %q", seen.authz)
	}
	if seen.apiKey != "az-secret" {
		t.Errorf("api-key=%q want 'az-secret'", seen.apiKey)
	}
}

func TestBuild_AzureURLBuildsFromResourceWhenAPIEmpty(t *testing.T) {
	// No `api` override → URL composed from resource name in env.
	// We can't reach api.azure.com from a unit test; verify by
	// inspecting the resulting Provider's pinned Endpoint via a
	// failed HTTP roundtrip's error message (catches the URL).
	entry := &catalog.Provider{
		ID: "azure", NPM: "@ai-sdk/azure",
		Env: []string{"AZURE_RESOURCE_NAME", "AZURE_API_KEY"}, API: "",
		Models: map[string]*catalog.Model{"my-deployment": {ID: "my-deployment"}},
	}
	prov, _, err := compat.Build(entry, "my-deployment",
		func(name string) string {
			switch name {
			case "AZURE_RESOURCE_NAME":
				return "my-resource"
			case "AZURE_API_KEY":
				return "az-secret"
			}
			return ""
		})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if prov.Name() != "azure" {
		t.Errorf("Name()=%q", prov.Name())
	}
	// Issue a Complete that will fail (network) but the error will
	// embed the URL we tried to reach — proves the URL was built
	// from the env'd resource.
	_, err = prov.Complete(context.Background(), agent.CompletionRequest{
		Model:    "my-deployment",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.Contains(err.Error(), "my-resource.openai.azure.com") {
		t.Errorf("error should mention the built URL; got %v", err)
	}
	if !strings.Contains(err.Error(), "my-deployment") {
		t.Errorf("error should mention the deployment; got %v", err)
	}
}

func TestBuild_AzureMissingApiKeyRefused(t *testing.T) {
	entry := &catalog.Provider{
		ID: "azure", NPM: "@ai-sdk/azure",
		Env:    []string{"AZURE_RESOURCE_NAME", "AZURE_API_KEY"},
		Models: map[string]*catalog.Model{"d": {ID: "d"}},
	}
	// Only resource set, no API key.
	_, _, err := compat.Build(entry, "d", func(name string) string {
		if name == "AZURE_RESOURCE_NAME" {
			return "r"
		}
		return ""
	})
	// The standard cred resolver picks the first non-empty (the
	// resource), so it gets past the credentials guard. The azure
	// case then re-resolves and notices the API key is missing.
	if !errors.Is(err, compat.ErrMissingCredentials) {
		t.Errorf("got %v want ErrMissingCredentials", err)
	}
}

func TestBuild_AzureMissingResourceAndNoAPIOverrideRefused(t *testing.T) {
	entry := &catalog.Provider{
		ID: "azure", NPM: "@ai-sdk/azure",
		Env: []string{"AZURE_RESOURCE_NAME", "AZURE_API_KEY"}, API: "",
		Models: map[string]*catalog.Model{"d": {ID: "d"}},
	}
	// Only API key set; no resource and no `api` override.
	_, _, err := compat.Build(entry, "d", func(name string) string {
		if name == "AZURE_API_KEY" {
			return "k"
		}
		return ""
	})
	if !errors.Is(err, compat.ErrMissingCredentials) {
		t.Errorf("got %v want ErrMissingCredentials", err)
	}
	if err == nil || !strings.Contains(err.Error(), "custom.json") {
		t.Errorf("error should mention custom.json escape hatch: %v", err)
	}
}

func TestBuild_CohereFamilyRoutesToCohereWire(t *testing.T) {
	var seen struct {
		path, auth string
		body       map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.path = r.URL.Path
		seen.auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&seen.body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "msg-x",
			"finish_reason": "COMPLETE",
			"message": map[string]any{
				"role":    "assistant",
				"content": []map[string]any{{"type": "text", "text": "hi from cohere"}},
			},
			"usage": map[string]any{"tokens": map[string]any{"input_tokens": 3, "output_tokens": 5}},
		})
	}))
	defer srv.Close()

	entry := &catalog.Provider{
		ID: "cohere", Name: "Cohere",
		Env: []string{"COHERE_API_KEY"},
		NPM: "@ai-sdk/cohere", API: srv.URL,
		Models: map[string]*catalog.Model{"command-r-plus": {ID: "command-r-plus"}},
	}
	prov, _, err := compat.Build(entry, "command-r-plus",
		func(name string) string {
			if name == "COHERE_API_KEY" {
				return "co-test"
			}
			return ""
		})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if prov.Name() != "cohere" {
		t.Errorf("Name()=%q want 'cohere'", prov.Name())
	}
	resp, err := prov.Complete(context.Background(), agent.CompletionRequest{
		Model:    "command-r-plus",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hi from cohere" {
		t.Errorf("content=%q", resp.Message.Content)
	}
	if seen.path != "/v2/chat" {
		t.Errorf("path=%q want /v2/chat", seen.path)
	}
	if seen.auth != "Bearer co-test" {
		t.Errorf("auth=%q want 'Bearer co-test'", seen.auth)
	}
	if seen.body["model"] != "command-r-plus" {
		t.Errorf("body.model=%v", seen.body["model"])
	}
}

func TestBuild_GoogleFamilyRoutesToGeminiWire(t *testing.T) {
	var seen struct {
		path, apiKey string
		body         map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.path = r.URL.Path
		seen.apiKey = r.Header.Get("x-goog-api-key")
		_ = json.NewDecoder(r.Body).Decode(&seen.body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content":      map[string]any{"role": "model", "parts": []map[string]any{{"text": "hi from gemini"}}},
				"finishReason": "STOP",
			}},
			"usageMetadata": map[string]any{"promptTokenCount": 2, "candidatesTokenCount": 4},
		})
	}))
	defer srv.Close()

	entry := &catalog.Provider{
		ID: "google", Name: "Google",
		Env: []string{"GEMINI_API_KEY"},
		NPM: "@ai-sdk/google", API: srv.URL,
		Models: map[string]*catalog.Model{"gemini-1.5-flash": {ID: "gemini-1.5-flash"}},
	}
	prov, model, err := compat.Build(entry, "gemini-1.5-flash",
		func(name string) string {
			if name == "GEMINI_API_KEY" {
				return "g-key"
			}
			return ""
		})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if model != "gemini-1.5-flash" {
		t.Errorf("model=%q", model)
	}
	if prov.Name() != "google" {
		t.Errorf("Name()=%q want 'google'", prov.Name())
	}
	resp, err := prov.Complete(context.Background(), agent.CompletionRequest{
		Model:    "gemini-1.5-flash",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hi from gemini" {
		t.Errorf("response: %q", resp.Message.Content)
	}
	if seen.path != "/v1beta/models/gemini-1.5-flash:generateContent" {
		t.Errorf("path=%q want /v1beta/models/gemini-1.5-flash:generateContent", seen.path)
	}
	if seen.apiKey != "g-key" {
		t.Errorf("x-goog-api-key=%q want g-key", seen.apiKey)
	}
	// Verify the model id is in the URL path (Gemini quirk), not the body.
	if seen.body["model"] != nil {
		t.Errorf("body should not carry model (Gemini puts it in the path); got %v", seen.body["model"])
	}
}

func TestBuild_MissingCredsReturnsErr(t *testing.T) {
	entry := newAnthCatalogEntry("https://api.anthropic.com")
	_, _, err := compat.Build(entry, "claude-opus-4-7", func(string) string { return "" })
	if !errors.Is(err, compat.ErrMissingCredentials) {
		t.Errorf("got %v want ErrMissingCredentials", err)
	}
}

func TestBuild_MissingCredsWithNilLookupReturnsErr(t *testing.T) {
	entry := newAnthCatalogEntry("https://api.anthropic.com")
	_, _, err := compat.Build(entry, "claude-opus-4-7", nil)
	if !errors.Is(err, compat.ErrMissingCredentials) {
		t.Errorf("got %v want ErrMissingCredentials", err)
	}
}

func TestBuild_UnknownModelReturnsErr(t *testing.T) {
	entry := newAnthCatalogEntry("https://api.anthropic.com")
	_, _, err := compat.Build(entry, "model-that-doesnt-exist", func(string) string { return "k" })
	if !errors.Is(err, compat.ErrModelUnknown) {
		t.Errorf("got %v want ErrModelUnknown", err)
	}
}

func TestBuild_NilProviderReturnsErr(t *testing.T) {
	if _, _, err := compat.Build(nil, "x", nil); err == nil {
		t.Error("expected error for nil provider")
	}
}

func TestBuild_MistralRoutesThroughOpenAIWire(t *testing.T) {
	var seen struct {
		auth, path string
		body       map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.auth = r.Header.Get("Authorization")
		seen.path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&seen.body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "cmpl-m", "model": "mistral-small-latest",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "bonjour"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 3},
		})
	}))
	defer srv.Close()

	entry := &catalog.Provider{
		ID: "mistral", Name: "Mistral",
		Env: []string{"MISTRAL_API_KEY"},
		NPM: "@ai-sdk/mistral",
		API: srv.URL + "/v1", // simulate operator override; default would point at api.mistral.ai
		Models: map[string]*catalog.Model{
			"mistral-small-latest": {ID: "mistral-small-latest"},
		},
	}
	prov, _, err := compat.Build(entry, "mistral-small-latest",
		func(name string) string {
			if name == "MISTRAL_API_KEY" {
				return "ms-test"
			}
			return ""
		})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if prov.Name() != "mistral" {
		t.Errorf("Name()=%q want 'mistral'", prov.Name())
	}
	resp, err := prov.Complete(context.Background(), agent.CompletionRequest{
		Model:    "mistral-small-latest",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "salut"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "bonjour" {
		t.Errorf("content=%q", resp.Message.Content)
	}
	if seen.path != "/v1/chat/completions" {
		t.Errorf("path=%q want /v1/chat/completions", seen.path)
	}
	if seen.auth != "Bearer ms-test" {
		t.Errorf("auth=%q", seen.auth)
	}
	if seen.body["model"] != "mistral-small-latest" {
		t.Errorf("body.model=%v", seen.body["model"])
	}
}

func TestBuild_MistralDefaultBaseURLWhenAPIEmpty(t *testing.T) {
	// Build a Mistral entry with NO api field. compat must fill in
	// the api.mistral.ai default rather than falling back to the
	// openai adapter's api.openai.com default. We can't actually hit
	// the public host in a unit test, so we just verify Build
	// succeeds (it would error with ErrFamilyUnsupported if compat
	// treated empty api as a catalog-compat refusal like it does
	// for openai-compatible).
	entry := &catalog.Provider{
		ID: "mistral", NPM: "@ai-sdk/mistral",
		Env: []string{"MISTRAL_API_KEY"}, API: "",
		Models: map[string]*catalog.Model{"m": {ID: "m"}},
	}
	prov, _, err := compat.Build(entry, "m", func(string) string { return "k" })
	if err != nil {
		t.Fatalf("Build with empty api: %v", err)
	}
	if prov.Name() != "mistral" {
		t.Errorf("Name()=%q", prov.Name())
	}
}

func TestBuild_OpenAICompatibleEmptyAPIRefused(t *testing.T) {
	// An openai-compatible entry with no api URL would silently
	// inherit api.openai.com — wrong vendor. compat must refuse.
	entry := &catalog.Provider{
		ID: "groq", NPM: "@ai-sdk/groq",
		Env: []string{"GROQ_API_KEY"}, API: "",
		Models: map[string]*catalog.Model{"m": {ID: "m"}},
	}
	_, _, err := compat.Build(entry, "m", func(string) string { return "k" })
	if !errors.Is(err, compat.ErrFamilyUnsupported) {
		t.Errorf("got %v want ErrFamilyUnsupported", err)
	}
	if err == nil || !strings.Contains(err.Error(), "custom.json") {
		t.Errorf("error should mention custom.json: %v", err)
	}
}

// ---- support flags ----

func TestIsSupportedFamily(t *testing.T) {
	cases := map[catalog.Family]bool{
		catalog.FamilyAnthropic:        true,
		catalog.FamilyOllama:           true,
		catalog.FamilyOpenAI:           true,
		catalog.FamilyOpenAICompatible: true,
		catalog.FamilyGoogle:           true,
		catalog.FamilyGoogleVertex:     true,
		catalog.FamilyMistral:          true,
		catalog.FamilyCohere:           true,
		catalog.FamilyAzure:            true,
		catalog.FamilyAWSBedrock:       true,
		catalog.FamilyUnknown:          false,
	}
	for f, want := range cases {
		if got := compat.IsSupportedFamily(f); got != want {
			t.Errorf("IsSupportedFamily(%s)=%v want %v", f, got, want)
		}
	}
}

// ---- FirstModelID ----

func TestFirstModelID_AlphabeticallySmallest(t *testing.T) {
	entry := &catalog.Provider{
		Models: map[string]*catalog.Model{
			"zeta":  {ID: "zeta"},
			"alpha": {ID: "alpha"},
			"mu":    {ID: "mu"},
		},
	}
	if got := compat.FirstModelID(entry); got != "alpha" {
		t.Errorf("FirstModelID=%q want alpha", got)
	}
}

func TestFirstModelID_EmptyOrNil(t *testing.T) {
	if compat.FirstModelID(nil) != "" {
		t.Error("nil → empty")
	}
	if compat.FirstModelID(&catalog.Provider{}) != "" {
		t.Error("no models → empty")
	}
}

// ---- streaming capability pass-through ----

// TestBuild_PreservesStreamingCapability locks in the M1.q.x
// wrapNamed contract: when the inner adapter implements
// agent.StreamingProvider (Anthropic in M1.q; OpenAI + family in
// M1.q.x), the wrapped provider returned by Build MUST still type-
// assert as a StreamingProvider. Otherwise the namedProvider wrapper
// silently dropped the capability — that bug would break
// `agt provider check --stream` for the very providers it was added
// to cover.
func TestBuild_PreservesStreamingCapability(t *testing.T) {
	cases := []struct {
		name string
		npm  string
		api  string
	}{
		{name: "anthropic", npm: "@ai-sdk/anthropic", api: "http://example.invalid"},
		{name: "openai", npm: "@ai-sdk/openai", api: "https://api.openai.com/v1"},
		{name: "groq (openai-compatible)", npm: "@ai-sdk/groq", api: "https://api.groq.com/openai/v1"},
		{name: "mistral", npm: "@ai-sdk/mistral", api: "https://api.mistral.ai/v1"},
		{name: "google (gemini)", npm: "@ai-sdk/google", api: "https://generativelanguage.googleapis.com"},
		{name: "cohere", npm: "@ai-sdk/cohere", api: "https://api.cohere.com"},
	}
	lookup := func(string) string { return "key" }
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entry := &catalog.Provider{
				ID: c.name, NPM: c.npm, API: c.api,
				Env:    []string{"API_KEY"},
				Models: map[string]*catalog.Model{"m": {ID: "m"}},
			}
			prov, _, err := compat.Build(entry, "m", lookup)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if _, ok := prov.(agent.StreamingProvider); !ok {
				t.Errorf("Build returned %T which does not implement agent.StreamingProvider — wrapper dropped the capability", prov)
			}
		})
	}
}

// TestBuild_VertexPreservesStreamingCapability is the
// special-cased twin of TestBuild_PreservesStreamingCapability for
// google-vertex, which can't be a table entry because building it
// requires a real RSA-signed service-account JSON on disk (the
// JWT-bearer flow validates the key at TokenSource construction).
// The single-case test reuses the same genTestVertexSA helper the
// existing Vertex Build tests use.
func TestBuild_VertexPreservesStreamingCapability(t *testing.T) {
	saJSON := genTestVertexSA(t, "http://token-uri.example.invalid")
	saPath := writeTempFile(t, saJSON)
	entry := &catalog.Provider{
		ID: "google-vertex", NPM: "@ai-sdk/google-vertex",
		Env: []string{"GOOGLE_VERTEX_PROJECT", "GOOGLE_VERTEX_LOCATION", "GOOGLE_APPLICATION_CREDENTIALS"},
		// Empty API → defaultBaseURL kicks in; we won't actually call
		// the URL, just check the type assertion.
		Models: map[string]*catalog.Model{"gemini-1.5-flash": {ID: "gemini-1.5-flash"}},
	}
	prov, _, err := compat.Build(entry, "gemini-1.5-flash", func(name string) string {
		switch name {
		case "GOOGLE_APPLICATION_CREDENTIALS":
			return saPath
		case "GOOGLE_VERTEX_LOCATION":
			return "us-central1"
		case "GOOGLE_VERTEX_PROJECT":
			return "my-proj"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := prov.(agent.StreamingProvider); !ok {
		t.Errorf("Build returned %T which does not implement agent.StreamingProvider — wrapper dropped the capability for google-vertex", prov)
	}
}

// TestBuild_OllamaPreservesStreamingCapability verifies the local
// Ollama wrapper retains its StreamingProvider capability through
// compat.Build. Kept separate from the table because Ollama has no
// `env` list (local server, no auth) and the helper closure can be
// trivial.
func TestBuild_OllamaPreservesStreamingCapability(t *testing.T) {
	entry := &catalog.Provider{
		ID: "ollama-local", NPM: "@ai-sdk/ollama",
		API:    "http://localhost:11434",
		Models: map[string]*catalog.Model{"llama3.2": {ID: "llama3.2"}},
	}
	prov, _, err := compat.Build(entry, "llama3.2", func(string) string { return "" })
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := prov.(agent.StreamingProvider); !ok {
		t.Errorf("Build returned %T which does not implement agent.StreamingProvider — wrapper dropped capability for ollama", prov)
	}
}

// TestBuild_BedrockAdvertisesStreaming verifies the wrapper exposes
// StreamingProvider for AWS Bedrock now that the binary event-stream
// parser ships (M1.t). Was previously a negative-guard ("must NOT
// advertise") — flipped when streaming.go landed.
func TestBuild_BedrockAdvertisesStreaming(t *testing.T) {
	entry := &catalog.Provider{
		ID: "aws-bedrock", NPM: "@ai-sdk/amazon-bedrock",
		Env:    []string{"AWS_BEARER_TOKEN_BEDROCK", "AWS_REGION"},
		Models: map[string]*catalog.Model{"anthropic.claude-3-5-sonnet-20241022-v2:0": {ID: "anthropic.claude-3-5-sonnet-20241022-v2:0"}},
	}
	prov, _, err := compat.Build(entry, "anthropic.claude-3-5-sonnet-20241022-v2:0", func(name string) string {
		switch name {
		case "AWS_BEARER_TOKEN_BEDROCK":
			return "fake-bearer"
		case "AWS_REGION":
			return "us-east-1"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := prov.(agent.StreamingProvider); !ok {
		t.Errorf("bedrock should advertise agent.StreamingProvider (M1.t shipped streaming), but did not")
	}
}

// ---- credential resolution: first non-empty wins ----

func TestBuild_AnyOfCredentialsWorks(t *testing.T) {
	entry := &catalog.Provider{
		ID: "any-of-creds", NPM: "@ai-sdk/anthropic",
		API:    "http://example.invalid",
		Env:    []string{"FIRST_KEY", "SECOND_KEY", "THIRD_KEY"},
		Models: map[string]*catalog.Model{"m": {ID: "m"}},
	}
	lookup := func(name string) string {
		if name == "SECOND_KEY" {
			return "found-it"
		}
		return ""
	}
	_, _, err := compat.Build(entry, "m", lookup)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
}
