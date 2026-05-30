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

// healthServer returns an httptest server mimicking a peer's REST /api/v1/health,
// requiring the given token (empty = no auth check).
func healthServer(t *testing.T, token, version string, models int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok", "version": version, "model_count": models,
		})
	}))
}

func TestPeers_HealthyPeer(t *testing.T) {
	srv := healthServer(t, "tok", "1.2.3", 7)
	defer srv.Close()
	t.Setenv("AGEZT_PEERS", "nodeB="+srv.URL+"|tok")

	var out, errb bytes.Buffer
	code := cmdPeers(nil, &out, &errb)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errb.String())
	}
	s := out.String()
	if !strings.Contains(s, "nodeB") || !strings.Contains(s, "OK") || !strings.Contains(s, "1.2.3") {
		t.Errorf("output missing healthy peer info:\n%s", s)
	}
}

func TestPeers_JSON(t *testing.T) {
	srv := healthServer(t, "", "9.9", 3)
	defer srv.Close()
	t.Setenv("AGEZT_PEERS", "x="+srv.URL)

	var out, errb bytes.Buffer
	if code := cmdPeers([]string{"--json"}, &out, &errb); code != 0 {
		t.Fatalf("exit=%d", code)
	}
	var arr []peerHealth
	if err := json.Unmarshal(out.Bytes(), &arr); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if len(arr) != 1 || !arr[0].Reachable || arr[0].Version != "9.9" || arr[0].ModelCount != 3 {
		t.Errorf("json result = %+v", arr)
	}
}

func TestPeers_BadToken(t *testing.T) {
	srv := healthServer(t, "right", "1.0", 1)
	defer srv.Close()
	t.Setenv("AGEZT_PEERS", "x="+srv.URL+"|wrong")

	var out, errb bytes.Buffer
	code := cmdPeers(nil, &out, &errb)
	if code != 1 { // unreachable → non-zero
		t.Errorf("bad token should exit 1, got %d", code)
	}
	if !strings.Contains(out.String(), "UNREACHABLE") || !strings.Contains(out.String(), "401") {
		t.Errorf("should report 401:\n%s", out.String())
	}
}

func TestPeers_NoneConfigured(t *testing.T) {
	t.Setenv("AGEZT_PEERS", "")
	var out, errb bytes.Buffer
	if code := cmdPeers(nil, &out, &errb); code != 0 {
		t.Errorf("no peers should exit 0, got %d", code)
	}
	if !strings.Contains(out.String(), "No peers configured") {
		t.Errorf("should say none configured:\n%s", out.String())
	}
}

func TestPeers_NoneConfiguredJSON(t *testing.T) {
	t.Setenv("AGEZT_PEERS", "")
	var out, errb bytes.Buffer
	cmdPeers([]string{"--json"}, &out, &errb)
	if strings.TrimSpace(out.String()) != "[]" {
		t.Errorf("empty JSON should be []: %q", out.String())
	}
}

func TestPeers_BadSpec(t *testing.T) {
	t.Setenv("AGEZT_PEERS", "broken-no-equals")
	var out, errb bytes.Buffer
	if code := cmdPeers(nil, &out, &errb); code != 1 {
		t.Errorf("malformed spec should exit 1, got %d", code)
	}
}
