// SPDX-License-Identifier: MIT

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPCoverageDefinitionAndHelpers(t *testing.T) {
	// Definition with empty allowlist.
	tool := New()
	def := tool.Definition()
	if def.Name != "http" {
		t.Fatalf("Name = %q", def.Name)
	}
	if def.Description == "" || len(def.InputSchema) == 0 {
		t.Fatalf("Definition = %+v", def)
	}
	if !strings.Contains(def.Effect.RollbackNotes, "GET requests need no rollback") {
		t.Fatalf("rollback note = %q", def.Effect.RollbackNotes)
	}

	// Definition with hosts.
	tool.AllowedHosts = []string{"example.com", "*.x.com"}
	def = tool.Definition()
	if !strings.Contains(def.Effect.AffectedResources[0], "example.com, *.x.com") {
		t.Fatalf("resources should list hosts, got %q", def.Effect.AffectedResources)
	}

	// Definition with AllowAll.
	tool.AllowAll = true
	def = tool.Definition()
	if !strings.Contains(def.Effect.AffectedResources[0], "all hosts") {
		t.Fatalf("resources should mention all hosts: %q", def.Effect.AffectedResources)
	}

	// hostAllowed: exact, wildcard, missing wildcard segment, AllowAll, empty.
	cases := map[string]bool{
		"example.com":  true,
		"sub.x.com":    true,
		"x.com":        false, // bare apex should NOT match *.x.com
		"deep.x.com":   true,
		"sub.evil.com": false,
		"evil.com":     false,
	}
	for host, want := range cases {
		tool2 := New()
		tool2.AllowedHosts = []string{"example.com", "*.x.com"}
		if got := tool2.hostAllowed(host); got != want {
			t.Fatalf("hostAllowed(%q) = %v, want %v", host, got, want)
		}
	}
	if !(&Tool{AllowAll: true}).hostAllowed("anything") {
		t.Fatal("AllowAll should accept any host")
	}
	if (&Tool{}).hostAllowed("example.com") {
		t.Fatal("empty allowlist should not allow any host")
	}

	// client() returns the injected client unchanged and does not call OnBlock.
	custom := &http.Client{}
	tool3 := New()
	tool3.HTTP = custom
	called := false
	tool3.OnBlock = func(_, _ string) { called = true }
	if got := tool3.client(); got != custom {
		t.Fatalf("client() = %p, want %p", got, custom)
	}
	if called {
		t.Fatal("OnBlock should not be called when HTTP is injected")
	}
}

func TestHTTPCoverageInvokeValidation(t *testing.T) {
	tool := New()
	tool.AllowAll = true

	// Parse error → hard error.
	_, err := tool.Invoke(context.Background(), json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("parse error = %v", err)
	}

	// Empty method.
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"url":"https://x"}`))
	if err != nil || !res.IsError || !strings.Contains(res.Output, "method required") {
		t.Fatalf("empty method = %+v err %v", res, err)
	}

	// Method not GET/POST.
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"method":"PUT","url":"https://x"}`))
	if err != nil || !res.IsError || !strings.Contains(res.Output, "PUT not allowed") {
		t.Fatalf("method PUT = %+v err %v", res, err)
	}

	// Invalid scheme.
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"method":"GET","url":"ftp://x"}`))
	if err != nil || !res.IsError || !strings.Contains(res.Output, "scheme") {
		t.Fatalf("ftp scheme = %+v err %v", res, err)
	}

	// Empty host.
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"method":"GET","url":"https:///path"}`))
	if err != nil || !res.IsError || !strings.Contains(res.Output, "missing host") {
		t.Fatalf("empty host = %+v err %v", res, err)
	}

	// Body too large.
	tool2 := New()
	tool2.AllowAll = true
	big := strings.Repeat("a", MaxRequestBodyBytes+1)
	res, err = tool2.Invoke(context.Background(), json.RawMessage(`{"method":"POST","url":"https://x","body":"`+big+`"}`))
	if err != nil || !res.IsError || !strings.Contains(res.Output, "body too large") {
		t.Fatalf("body too large = %+v err %v", res, err)
	}
}

func TestHTTPCoverageInvokeHappyAndStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	tool := New()
	tool.AllowAll = true
	tool.HTTP = srv.Client()

	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"method":"POST","url":"`+srv.URL+`","body":"hello"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got %q", res.Output)
	}
	if !strings.Contains(res.Output, `"status": 201`) || !strings.Contains(res.Output, `"body": "ok"`) {
		t.Fatalf("output missing status/body: %s", res.Output)
	}
	if res.ObservationSource != srv.URL {
		t.Fatalf("ObservationSource = %q", res.ObservationSource)
	}

	// 4xx → IsError.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv2.Close()
	tool2 := New()
	tool2.AllowAll = true
	tool2.HTTP = srv2.Client()
	res, err = tool2.Invoke(context.Background(), json.RawMessage(`{"method":"GET","url":"`+srv2.URL+`"}`))
	if err != nil {
		t.Fatalf("Invoke 4xx: %v", err)
	}
	if !res.IsError {
		t.Fatalf("4xx should be IsError, got %+v", res)
	}
}
