// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestRunRemoteExecutionProfileMirrorsPeerEventMetadata(t *testing.T) {
	peerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runs/run-abc" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer sekrit" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"correlation_id": "run-abc",
			"count":          2,
			"events": []map[string]any{
				{"id": "e1", "seq": 1, "ts_unix_ms": 1000, "subject": "agent.agent-run-abc.task", "actor": "agent-run-abc", "kind": "task.received", "correlation_id": "run-abc", "hash": "h1", "payload": map[string]any{"secret": "remote payload must not mirror"}},
				{"id": "e2", "seq": 2, "ts_unix_ms": 1001, "subject": "agent.agent-run-abc.task", "actor": "agent-run-abc", "kind": "task.completed", "correlation_id": "run-abc", "hash": "h2"},
			},
		})
	}))
	defer peerSrv.Close()
	t.Setenv("AGEZT_PEERS", "nodeB="+peerSrv.URL+"|sekrit")
	t.Setenv("AGEZT_REMOTE_EVENT_MIRROR", "metadata")

	_, _, c, _ := startPairWithConfig(t, runtime.Config{
		Provider: mock.New(mock.FinalText("ok")),
		Tools:    map[string]agent.Tool{"remote_run": remoteRunFooterTool{}},
	})
	var mirrored map[string]any
	_, err := c.Stream(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "delegate this", "execution_profile": "remote-agezt"}, func(e *event.Event) {
			if e.Kind != event.KindInfo {
				return
			}
			var p map[string]any
			_ = json.Unmarshal(e.Payload, &p)
			if p["phase"] == "peer_events_mirrored" {
				mirrored = p
			}
		})
	if err != nil {
		t.Fatalf("remote-agezt run errored: %v", err)
	}
	if mirrored["remote_peer"] != "nodeB" || mirrored["remote_correlation"] != "run-abc" || mirrored["mode"] != "metadata" {
		t.Fatalf("mirror payload = %v, want peer/correlation/mode metadata", mirrored)
	}
	if mirrored["count"] != float64(2) && mirrored["count"] != 2 {
		t.Fatalf("mirror count = %v, want 2", mirrored["count"])
	}
	raw, _ := json.Marshal(mirrored)
	if strings.Contains(string(raw), "sekrit") || strings.Contains(string(raw), "remote payload must not mirror") {
		t.Fatalf("mirror leaked token or remote payload: %s", raw)
	}
	if !strings.Contains(string(raw), "task.completed") {
		t.Fatalf("mirror should include remote event metadata: %s", raw)
	}
}

func TestRunRemoteExecutionProfileMirrorsRedactedPeerPayloads(t *testing.T) {
	const remoteSecret = "sk-abcdefghijklmnopqrstuvwxyz123456"
	peerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runs/run-abc" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"correlation_id": "run-abc",
			"events": []map[string]any{
				{
					"id": "e1", "seq": 1, "ts_unix_ms": 1000,
					"subject": "agent.agent-run-abc.task", "actor": "agent-run-abc",
					"kind": "task.completed", "correlation_id": "run-abc", "hash": "h1",
					"payload": map[string]any{"answer": "remote done", "api_key": remoteSecret},
				},
			},
		})
	}))
	defer peerSrv.Close()
	t.Setenv("AGEZT_PEERS", "nodeB="+peerSrv.URL)
	t.Setenv("AGEZT_REMOTE_EVENT_MIRROR", "redacted")

	_, _, c, _ := startPairWithConfig(t, runtime.Config{
		Provider: mock.New(mock.FinalText("ok")),
		Tools:    map[string]agent.Tool{"remote_run": remoteRunFooterTool{}},
	})
	var mirrored map[string]any
	_, err := c.Stream(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "delegate this", "execution_profile": "remote-agezt"}, func(e *event.Event) {
			if e.Kind != event.KindInfo {
				return
			}
			var p map[string]any
			_ = json.Unmarshal(e.Payload, &p)
			if p["phase"] == "peer_events_mirrored" {
				mirrored = p
			}
		})
	if err != nil {
		t.Fatalf("remote-agezt run errored: %v", err)
	}
	if mirrored["mode"] != "redacted" || mirrored["payload_mode"] != "redacted" {
		t.Fatalf("mirror payload = %v, want redacted mode", mirrored)
	}
	raw, _ := json.Marshal(mirrored)
	if strings.Contains(string(raw), remoteSecret) {
		t.Fatalf("redacted mirror leaked remote secret: %s", raw)
	}
	if !strings.Contains(string(raw), "payload_redacted") || !strings.Contains(string(raw), "[REDACTED]") || !strings.Contains(string(raw), "remote done") {
		t.Fatalf("redacted mirror missing payload summary/redaction: %s", raw)
	}
}

func TestRunRemoteExecutionProfileMirrorsPeerArtifactMetadata(t *testing.T) {
	peerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/runs/run-abc":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"correlation_id": "run-abc",
				"events": []map[string]any{
					{"id": "e1", "seq": 1, "ts_unix_ms": 1000, "subject": "agent.agent-run-abc.task", "actor": "agent-run-abc", "kind": "task.completed", "correlation_id": "run-abc", "hash": "h1"},
				},
			})
		case "/api/v1/artifacts":
			if got := r.URL.Query().Get("corr"); got != "run-abc" {
				t.Errorf("artifact corr query = %q, want run-abc", got)
			}
			if got := r.URL.Query().Get("limit"); got == "" {
				t.Errorf("artifact limit query missing")
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
						"data": "remote raw bytes must not survive decode",
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer peerSrv.Close()
	t.Setenv("AGEZT_PEERS", "nodeB="+peerSrv.URL+"|sekrit")
	t.Setenv("AGEZT_REMOTE_EVENT_MIRROR", "metadata")

	_, _, c, _ := startPairWithConfig(t, runtime.Config{
		Provider: mock.New(mock.FinalText("ok")),
		Tools:    map[string]agent.Tool{"remote_run": remoteRunFooterTool{}},
	})
	var mirrored map[string]any
	_, err := c.Stream(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "delegate this", "execution_profile": "remote-agezt"}, func(e *event.Event) {
			if e.Kind != event.KindInfo {
				return
			}
			var p map[string]any
			_ = json.Unmarshal(e.Payload, &p)
			if p["phase"] == "peer_events_mirrored" {
				mirrored = p
			}
		})
	if err != nil {
		t.Fatalf("remote-agezt run errored: %v", err)
	}
	if mirrored["artifact_count"] != float64(1) && mirrored["artifact_count"] != 1 {
		t.Fatalf("artifact_count = %v, want 1 in %v", mirrored["artifact_count"], mirrored)
	}
	raw, _ := json.Marshal(mirrored)
	for _, forbidden := range []string{"sekrit", "remote raw bytes must not survive decode", `"data"`} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("artifact mirror leaked forbidden field/value %q: %s", forbidden, raw)
		}
	}
	for _, want := range []string{"artifacts", "art-1", strings.Repeat("a", 64), "result.txt"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("artifact mirror missing %q: %s", want, raw)
		}
	}
}
