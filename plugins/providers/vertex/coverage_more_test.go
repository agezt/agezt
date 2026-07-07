// SPDX-License-Identifier: MIT

package vertex

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

type staticMinter string

func (s staticMinter) Token(context.Context) (string, error) { return string(s), nil }

func TestVertexCoverageIdentityErrorsAndLoadServiceAccount(t *testing.T) {
	p := New(staticMinter("tok"), "p", "loc")
	if p.Name() != "google-vertex" {
		t.Fatalf("Name = %q", p.Name())
	}
	if got := (&APIError{Status: 403, Body: "denied"}).Error(); !strings.Contains(got, "403") || !strings.Contains(got, "denied") {
		t.Fatalf("APIError = %q", got)
	}
	if _, err := LoadServiceAccountFile(""); err == nil || !strings.Contains(err.Error(), "path is empty") {
		t.Fatalf("empty load error = %v", err)
	}
	if _, err := LoadServiceAccountFile(filepath.Join(t.TempDir(), "missing.json")); err == nil || !strings.Contains(err.Error(), "read service account") {
		t.Fatalf("missing load error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(path, []byte(`{"type":"service_account","client_email":"u@example.com","private_key":"-----BEGIN-----\n-----END-----"}`), 0o600); err != nil {
		t.Fatalf("write sa: %v", err)
	}
	sa, err := LoadServiceAccountFile(path)
	if err != nil {
		t.Fatalf("LoadServiceAccountFile: %v", err)
	}
	if sa.ClientEmail != "u@example.com" || sa.TokenURI == "" {
		t.Fatalf("service account = %+v", sa)
	}
}

func TestVertexCoverageCompleteValidationAndAPIError(t *testing.T) {
	if _, err := New(staticMinter("tok"), "", "loc").Complete(context.Background(), agent.CompletionRequest{Model: "m"}); err == nil || !strings.Contains(err.Error(), "Project required") {
		t.Fatalf("missing project error = %v", err)
	}
	if _, err := New(staticMinter("tok"), "p", "").Complete(context.Background(), agent.CompletionRequest{Model: "m"}); err == nil || !strings.Contains(err.Error(), "Location required") {
		t.Fatalf("missing location error = %v", err)
	}
	if _, err := New(staticMinter("tok"), "p", "loc").Complete(context.Background(), agent.CompletionRequest{}); err != ErrNoModel {
		t.Fatalf("missing model error = %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("quota"))
	}))
	defer srv.Close()
	p := New(staticMinter("tok"), "p", "loc")
	p.Endpoint = srv.URL
	p.HTTP = srv.Client()
	_, err := p.Complete(context.Background(), agent.CompletionRequest{Model: "gemini", Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}}})
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != http.StatusTooManyRequests || !strings.Contains(apiErr.Body, "quota") {
		t.Fatalf("API error = %#v", err)
	}
}

func TestVertexCoverageNativeTranslationAndDecodeEdges(t *testing.T) {
	if c, err := canonicalToVertex(agent.Message{Role: agent.RoleSystem, Content: "ignored"}, nil); err != nil || c != nil {
		t.Fatalf("system canonical = %#v err %v", c, err)
	}
	assistant, err := canonicalToVertex(agent.Message{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{Name: "tool"}}}, map[string]string{"tool": "wire_tool"})
	if err != nil {
		t.Fatalf("assistant canonical: %v", err)
	}
	if assistant.Role != "model" || len(assistant.Parts) != 1 || assistant.Parts[0].FunctionCall.Name != "wire_tool" || string(assistant.Parts[0].FunctionCall.Args) != "{}" {
		t.Fatalf("assistant canonical = %+v", assistant)
	}
	if _, err := canonicalToVertex(agent.Message{Role: agent.RoleTool, Content: "out"}, nil); err == nil || !strings.Contains(err.Error(), "tool_call_id") {
		t.Fatalf("tool without id = %v", err)
	}
	if _, err := canonicalToVertex(agent.Message{Role: "alien", Content: "x"}, nil); err == nil || !strings.Contains(err.Error(), "unknown role") {
		t.Fatalf("unknown role = %v", err)
	}

	body, err := encodeRequest("system", []agent.Message{{Role: agent.RoleUser, Content: "hi"}}, []agent.ToolDef{{Name: "plain"}}, 7, true, -1, agent.Params{}, json.RawMessage(`{"labels":{"test":"yes"}}`))
	if err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	text := string(body)
	for _, want := range []string{"systemInstruction", "responseMimeType", "thinkingConfig", "functionDeclarations", `"labels":{"test":"yes"}`} {
		if !strings.Contains(text, want) {
			t.Fatalf("encoded request missing %q in %s", want, text)
		}
	}

	if _, err := decodeResponse([]byte(`not-json`), "m"); err == nil || !strings.Contains(err.Error(), "parse response") {
		t.Fatalf("bad json decode = %v", err)
	}
	if _, err := decodeResponse([]byte(`{"candidates":[]}`), "m"); err == nil || !strings.Contains(err.Error(), "no candidates") {
		t.Fatalf("no candidates decode = %v", err)
	}
	resp, err := decodeResponse([]byte(`{"candidates":[{"finishReason":"MAX_TOKENS","content":{"parts":[{"thought":true,"text":"reason"},{"text":"answer"},{"functionCall":{"name":"lookup","args":{}}}]} }],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"thoughtsTokenCount":4}}`), "gemini")
	if err != nil {
		t.Fatalf("decodeResponse: %v", err)
	}
	if resp.StopReason != agent.StopToolUse || resp.Message.Content != "answer" || resp.ReasoningContent != "reason" || resp.Usage.OutputTokens != 7 || len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("decoded response = %+v", resp)
	}
}

func generateTestSAJSONForCoverage(t *testing.T, tokenURI string) []byte {
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
	sa := ServiceAccountKey{
		Type:         "service_account",
		ProjectID:    "test-project",
		PrivateKey:   string(pemBytes),
		PrivateKeyID: "test-kid",
		ClientEmail:  "test@test-project.iam.gserviceaccount.com",
		TokenURI:     tokenURI,
	}
	raw, err := json.Marshal(sa)
	if err != nil {
		t.Fatalf("marshal sa json: %v", err)
	}
	return raw
}

func TestVertexCoverageTokenExchangeMissingAccessToken(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"expires_in": 3600})
	}))
	defer tokenSrv.Close()
	keyJSON := generateTestSAJSONForCoverage(t, tokenSrv.URL)
	sa, err := ParseServiceAccountJSON(keyJSON)
	if err != nil {
		t.Fatalf("parse sa: %v", err)
	}
	ts, err := NewTokenSource(sa, "", tokenSrv.Client())
	if err != nil {
		t.Fatalf("NewTokenSource: %v", err)
	}
	if _, err := ts.Token(context.Background()); err == nil || !strings.Contains(err.Error(), "missing access_token") {
		t.Fatalf("missing access token error = %v", err)
	}
}
