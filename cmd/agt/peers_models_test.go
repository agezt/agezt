// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// modelsServer returns an httptest server mimicking a peer's REST /api/v1/models,
// requiring the given token (empty = no auth check).
func modelsServer(t *testing.T, token, def string, models []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"default": def, "models": models})
	}))
}

func TestPeers_Models_JSON(t *testing.T) {
	srv := modelsServer(t, "tok", "sonnet", []string{"sonnet", "opus", "haiku"})
	defer srv.Close()
	t.Setenv("AGEZT_PEERS", "nodeB="+srv.URL+"|tok")

	var out, errb bytes.Buffer
	if code := cmdPeers([]string{"models", "--json"}, &out, &errb); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errb.String())
	}
	var arr []peerModels
	if err := json.Unmarshal(out.Bytes(), &arr); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if len(arr) != 1 || !arr[0].Reachable || arr[0].Default != "sonnet" {
		t.Fatalf("result = %+v", arr)
	}
	if strings.Join(arr[0].Models, ",") != "sonnet,opus,haiku" {
		t.Errorf("models = %v", arr[0].Models)
	}
}

func TestPeers_Models_Text(t *testing.T) {
	srv := modelsServer(t, "", "gpt-4o", []string{"gpt-4o", "gpt-4o-mini"})
	defer srv.Close()
	t.Setenv("AGEZT_PEERS", "edge="+srv.URL)

	var out, errb bytes.Buffer
	if code := cmdPeers([]string{"models"}, &out, &errb); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errb.String())
	}
	s := out.String()
	if !strings.Contains(s, "edge") || !strings.Contains(s, "default=gpt-4o") || !strings.Contains(s, "gpt-4o-mini") {
		t.Errorf("text output missing model info:\n%s", s)
	}
}

// TestPeers_Models_ByName filters to a single peer; the other peer is not queried.
func TestPeers_Models_ByName(t *testing.T) {
	srvA := modelsServer(t, "", "a-model", []string{"a-model"})
	defer srvA.Close()
	// Point peer B at a dead address — selecting A by name must not touch B.
	t.Setenv("AGEZT_PEERS", "A="+srvA.URL+",B=http://127.0.0.1:0")

	var out, errb bytes.Buffer
	if code := cmdPeers([]string{"models", "A", "--json"}, &out, &errb); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errb.String())
	}
	var arr []peerModels
	if err := json.Unmarshal(out.Bytes(), &arr); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(arr) != 1 || arr[0].Name != "A" || !arr[0].Reachable {
		t.Errorf("by-name result = %+v", arr)
	}
}

func TestPeers_Models_UnknownName(t *testing.T) {
	srv := modelsServer(t, "", "m", nil)
	defer srv.Close()
	t.Setenv("AGEZT_PEERS", "real="+srv.URL)

	var out, errb bytes.Buffer
	if code := cmdPeers([]string{"models", "ghost"}, &out, &errb); code != 1 {
		t.Fatalf("unknown peer should exit 1, got %d", code)
	}
	if !strings.Contains(errb.String(), "unknown peer") {
		t.Errorf("stderr = %q", errb.String())
	}
}

// TestPeers_NameOnlyValidWithModels guards the parser: a bare peer name without the
// `models` verb is rejected (it would be meaningless for `list`).
func TestPeers_NameOnlyValidWithModels(t *testing.T) {
	var out, errb bytes.Buffer
	if code := cmdPeers([]string{"someNode"}, &out, &errb); code != 2 {
		t.Fatalf("bare name should exit 2, got %d", code)
	}
	if !strings.Contains(errb.String(), "only valid with") {
		t.Errorf("stderr = %q", errb.String())
	}
}

// TestPeers_Models_OversizedBody proves fetchPeerModels bounds the response the same
// way checkPeer does (M201): an unbounded models body is cut off and the peer is
// reported unreachable rather than ingested.
func TestPeers_Models_OversizedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"default":"`))
		_, _ = w.Write(bytes.Repeat([]byte("x"), maxPeerResponseBytes+(1<<20)))
		_, _ = w.Write([]byte(`","models":[]}`))
	}))
	defer srv.Close()
	t.Setenv("AGEZT_PEERS", "big="+srv.URL)

	var out, errb bytes.Buffer
	_ = cmdPeers([]string{"models", "--json"}, &out, &errb)
	var arr []peerModels
	if err := json.Unmarshal(out.Bytes(), &arr); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if len(arr) != 1 || arr[0].Reachable {
		t.Fatalf("oversized models body must be rejected, got %+v", arr)
	}
	if !strings.Contains(arr[0].Error, "bad models response") {
		t.Errorf("want a decode error, got %q", arr[0].Error)
	}
}
