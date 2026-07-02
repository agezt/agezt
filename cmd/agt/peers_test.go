// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestPeers_Run_MetadataOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runs/run-abc" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"correlation_id": "run-abc",
			"count":          2,
			"events": []map[string]any{
				{"id": "e1", "seq": 1, "ts_unix_ms": 1, "subject": "agent.agent-run-abc.task", "actor": "agent-run-abc", "kind": "task.received", "correlation_id": "run-abc", "hash": "abc1234567890", "payload": map[string]any{"secret": "do-not-print"}},
				{"id": "e2", "seq": 2, "ts_unix_ms": 2, "subject": "agent.agent-run-abc.task", "actor": "agent-run-abc", "kind": "task.completed", "correlation_id": "run-abc", "hash": "def1234567890"},
			},
		})
	}))
	defer srv.Close()
	t.Setenv("AGEZT_PEERS", "nodeB="+srv.URL+"|tok")

	var out, errb bytes.Buffer
	if code := cmdPeers([]string{"run", "nodeB", "run-abc"}, &out, &errb); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errb.String())
	}
	s := out.String()
	if !strings.Contains(s, "peer nodeB run run-abc") || !strings.Contains(s, "task.completed") {
		t.Fatalf("run output missing event metadata:\n%s", s)
	}
	if strings.Contains(s, "tok") || strings.Contains(s, "do-not-print") {
		t.Fatalf("run output leaked token or payload:\n%s", s)
	}

	out.Reset()
	errb.Reset()
	if code := cmdPeers([]string{"run", "nodeB", "run-abc", "--json"}, &out, &errb); code != 0 {
		t.Fatalf("json exit=%d stderr=%s", code, errb.String())
	}
	if strings.Contains(out.String(), "tok") || strings.Contains(out.String(), "do-not-print") {
		t.Fatalf("json output leaked token or payload:\n%s", out.String())
	}
	var arc peerRunArc
	if err := json.Unmarshal(out.Bytes(), &arc); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if arc.Peer != "nodeB" || arc.CorrelationID != "run-abc" || len(arc.Events) != 2 {
		t.Fatalf("arc = %+v", arc)
	}
}

func TestPeers_Artifacts_MetadataOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/artifacts" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if got := r.URL.Query().Get("corr"); got != "run-abc" {
			t.Errorf("corr query = %q, want run-abc", got)
		}
		if got := r.URL.Query().Get("limit"); got == "" {
			t.Errorf("limit query missing")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"count":       1,
			"total_count": 1,
			"truncated":   false,
			"entries": []map[string]any{
				{
					"id": "art-1", "ref": strings.Repeat("a", 64), "name": "result.txt",
					"mime": "text/plain", "kind": "tool-output", "source": "run",
					"corr": "run-abc", "size": 12, "created_ms": 1002,
					"data": "do-not-print",
				},
			},
		})
	}))
	defer srv.Close()
	t.Setenv("AGEZT_PEERS", "nodeB="+srv.URL+"|tok")

	var out, errb bytes.Buffer
	if code := cmdPeers([]string{"artifacts", "nodeB", "run-abc"}, &out, &errb); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errb.String())
	}
	s := out.String()
	if !strings.Contains(s, "peer nodeB artifacts for run-abc") || !strings.Contains(s, "art-1") || !strings.Contains(s, "result.txt") {
		t.Fatalf("artifact output missing metadata:\n%s", s)
	}
	if strings.Contains(s, "tok") || strings.Contains(s, "do-not-print") {
		t.Fatalf("artifact output leaked token or raw bytes:\n%s", s)
	}

	out.Reset()
	errb.Reset()
	if code := cmdPeers([]string{"artifacts", "nodeB", "run-abc", "--json"}, &out, &errb); code != 0 {
		t.Fatalf("json exit=%d stderr=%s", code, errb.String())
	}
	if strings.Contains(out.String(), "tok") || strings.Contains(out.String(), "do-not-print") || strings.Contains(out.String(), `"data"`) {
		t.Fatalf("json artifact output leaked forbidden field/value:\n%s", out.String())
	}
	var list peerArtifactList
	if err := json.Unmarshal(out.Bytes(), &list); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if list.Peer != "nodeB" || list.CorrelationID != "run-abc" || len(list.Entries) != 1 || list.Entries[0].ID != "art-1" {
		t.Fatalf("artifact list = %+v", list)
	}
}

func TestPeers_ArtifactGet_WritesPolicyGatedBytes(t *testing.T) {
	const payload = "remote artifact bytes"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/artifacts/art-1/bytes" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entry": map[string]any{
				"id": "art-1", "ref": strings.Repeat("a", 64), "name": "result.txt",
				"mime": "text/plain", "kind": "tool-output", "corr": "run-abc",
				"size": len(payload), "created_ms": 1002,
			},
			"size": len(payload),
			"data": base64.StdEncoding.EncodeToString([]byte(payload)),
		})
	}))
	defer srv.Close()
	t.Setenv("AGEZT_PEERS", "nodeB="+srv.URL+"|tok")

	outFile := filepath.Join(t.TempDir(), "artifact.txt")
	var out, errb bytes.Buffer
	if code := cmdPeers([]string{"artifact-get", "nodeB", "art-1", outFile}, &out, &errb); code != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", code, errb.String(), out.String())
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(data) != payload {
		t.Fatalf("output file = %q, want payload", data)
	}
	s := out.String()
	if !strings.Contains(s, "wrote") || strings.Contains(s, "tok") || strings.Contains(s, payload) {
		t.Fatalf("artifact-get output should summarize without leaking token/raw bytes:\n%s", s)
	}
}
