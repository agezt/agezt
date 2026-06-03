// SPDX-License-Identifier: MIT

package peer

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/restapi"
)

// meshEngine is a minimal restapi.Engine standing in for a peer node's kernel:
// it echoes the task back as the "answer" so the round-trip is observable.
type meshEngine struct{ lastIntent string }

func (m *meshEngine) NewCorrelation() string        { return "run-peer-1" }
func (m *meshEngine) SubjectForRun(c string) string { return "agent.agent-" + c + ".llm" }
func (m *meshEngine) DefaultModel() string          { return "peer-model" }
func (m *meshEngine) ModelIDs() []string            { return []string{"peer-model"} }
func (m *meshEngine) RunModel(_ context.Context, _, intent, _ string, _ []string) (string, error) {
	m.lastIntent = intent
	return "peer handled: " + intent, nil
}
func (m *meshEngine) EventsForCorrelation(string) ([]*event.Event, error) { return nil, nil }

// TestLive_MeshRoundTripThroughRealRESTHandler drives the peer tool's real HTTP
// path (httpPost) against a real kernel/restapi server — the same handler the
// daemon mounts — so the mesh primitive is proven end to end over the wire,
// Bearer auth and all, not just against a fake poster.
func TestLive_MeshRoundTripThroughRealRESTHandler(t *testing.T) {
	// Peer node B: a real REST server backed by the mesh engine.
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })

	eng := &meshEngine{}
	restSrv := restapi.New(eng, b, "mesh-token", "0.0.0")
	ts := httptest.NewServer(restSrv.Handler())
	defer ts.Close()

	// Node A: the peer tool with the REAL httpPost, pointed at node B.
	tool := &Tool{
		Peers: map[string]Peer{"nodeB": {Name: "nodeB", URL: ts.URL, Token: "mesh-token"}},
		post:  httpPost,
	}

	in, _ := json.Marshal(map[string]string{"peer": "nodeB", "task": "add 2 and 2"})
	res, err := tool.Invoke(context.Background(), in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("mesh round-trip errored: %s", res.Output)
	}
	if !strings.Contains(res.Output, "peer handled: add 2 and 2") {
		t.Errorf("peer answer not relayed: %s", res.Output)
	}
	if !strings.Contains(res.Output, "peer=nodeB") || !strings.Contains(res.Output, "correlation=run-peer-1") {
		t.Errorf("footer missing: %s", res.Output)
	}
	if eng.lastIntent != "add 2 and 2" {
		t.Errorf("peer engine saw intent %q", eng.lastIntent)
	}
}

// TestLive_MeshRejectsBadToken proves the Bearer auth is really enforced across
// the wire: a wrong token yields a 401 the tool surfaces as an error.
func TestLive_MeshRejectsBadToken(t *testing.T) {
	j, _ := journal.Open(t.TempDir(), journal.Options{})
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })

	restSrv := restapi.New(&meshEngine{}, b, "right-token", "0.0.0")
	ts := httptest.NewServer(restSrv.Handler())
	defer ts.Close()

	tool := &Tool{
		Peers: map[string]Peer{"nodeB": {Name: "nodeB", URL: ts.URL, Token: "WRONG"}},
		post:  httpPost,
	}
	in, _ := json.Marshal(map[string]string{"peer": "nodeB", "task": "go"})
	res, _ := tool.Invoke(context.Background(), in)
	if !res.IsError {
		t.Error("a bad peer token should produce an error result")
	}
}
