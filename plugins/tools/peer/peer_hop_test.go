// SPDX-License-Identifier: MIT

package peer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/agezt/agezt/kernel/meshctx"
)

// TestRemoteRun_RefusesAtHopLimit: a run already at the hop limit refuses to delegate
// further (it would be refused by the peer anyway), without making a request.
func TestRemoteRun_RefusesAtHopLimit(t *testing.T) {
	called := false
	tool := &Tool{
		Peers: map[string]Peer{"a": {Name: "a", URL: "http://h:1"}},
		post: func(_ context.Context, _, _ string, _ []byte) (int, []byte, error) {
			called = true
			return 200, []byte(`{}`), nil
		},
	}
	ctx := meshctx.WithHop(context.Background(), meshctx.MaxHops)
	in, _ := json.Marshal(map[string]string{"task": "x"})
	res, err := tool.Invoke(ctx, in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError {
		t.Fatal("at the hop limit, remote_run should refuse")
	}
	if called {
		t.Error("must not POST when refusing at the hop limit")
	}
}

// TestHttpPost_ForwardsHopPlusOne: the default poster sends the current run's hop +1
// in the mesh-hop header so the peer can enforce the bound.
func TestHttpPost_ForwardsHopPlusOne(t *testing.T) {
	var gotHop string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHop = r.Header.Get(meshctx.HopHeader)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	ctx := meshctx.WithHop(context.Background(), 3)
	if _, _, err := httpPost(ctx, srv.URL, "", []byte(`{"intent":"x"}`)); err != nil {
		t.Fatalf("httpPost: %v", err)
	}
	if gotHop != strconv.Itoa(4) {
		t.Errorf("forwarded hop = %q, want \"4\" (3+1)", gotHop)
	}
}

// TestHttpPost_HopFromZero: a non-delegated run (hop 0) forwards hop 1.
func TestHttpPost_HopFromZero(t *testing.T) {
	var gotHop string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHop = r.Header.Get(meshctx.HopHeader)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	if _, _, err := httpPost(context.Background(), srv.URL, "", []byte(`{}`)); err != nil {
		t.Fatalf("httpPost: %v", err)
	}
	if gotHop != "1" {
		t.Errorf("forwarded hop = %q, want \"1\"", gotHop)
	}
}
