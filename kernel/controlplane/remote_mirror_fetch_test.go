// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appnodes "github.com/agezt/agezt/kernel/app/nodes"
	appremote "github.com/agezt/agezt/kernel/app/remote"
	"github.com/agezt/agezt/kernel/event"
)

// TestRemoteFetchesDecodeIntoTheApplicationRows: the host's peer fetches read
// the peer's answer into the application's own rows — the contract row and
// the canonical event — without a field-by-field projection, so a field the
// peer sends arrives with the value the peer gave it.
func TestRemoteFetchesDecodeIntoTheApplicationRows(t *testing.T) {
	peerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/runs/run-abc":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"correlation_id": "run-abc",
				"count":          1,
				"events": []map[string]any{{
					"id": "e1", "seq": 7, "ts_unix_ms": 1000,
					"subject": "agent.agent-run-abc.task", "actor": "agent-run-abc",
					"kind": "task.received", "correlation_id": "run-abc", "hash": "h1",
					"payload": map[string]any{"k": "v"},
				}},
			})
		case "/api/v1/artifacts":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"entries": []map[string]any{{
					"id": "art-1", "ref": "ref-1", "name": "result.txt", "mime": "text/plain",
					"kind": "tool-output", "source": "run", "sender": "agent-run-abc",
					"corr": "run-abc", "size": 11, "created_ms": 1000, "caption": "note",
				}},
				"truncated": true,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer peerSrv.Close()
	peer := appnodes.Peer{Name: "nodeB", URL: peerSrv.URL, Token: "sekrit"}

	events, err := fetchRemoteEvents(context.Background(), peer, "run-abc")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %v, want one", events)
	}
	ev := events[0]
	if ev.ID != "e1" || ev.Seq != 7 || ev.TSUnixMS != 1000 || ev.Subject != "agent.agent-run-abc.task" ||
		ev.Actor != "agent-run-abc" || ev.Kind != event.Kind("task.received") || ev.CorrelationID != "run-abc" ||
		ev.Hash != "h1" || string(ev.Payload) != `{"k":"v"}` {
		t.Fatalf("the peer's event did not arrive as it was sent: %+v", ev)
	}

	artifacts, truncated, err := fetchRemoteArtifacts(context.Background(), peer, "run-abc")
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("the peer's truncation flag is lost")
	}
	if len(artifacts) != 1 {
		t.Fatalf("artifacts = %v, want one", artifacts)
	}
	a := artifacts[0]
	if a.ID != "art-1" || a.Ref != "ref-1" || a.Name != "result.txt" || a.Mime != "text/plain" ||
		a.Kind != "tool-output" || a.Source != "run" || a.Sender != "agent-run-abc" || a.Corr != "run-abc" ||
		a.Size != 11 || a.CreatedMS != 1000 || a.Caption != "note" {
		t.Fatalf("the peer's artifact did not arrive as it was sent: %+v", a)
	}
	// The rows are the application's own types, so the host hands them over
	// without a conversion.
	var _ []appremote.Event = events
	var _ []appremote.Artifact = artifacts
}
