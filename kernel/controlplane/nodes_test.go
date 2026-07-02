// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestNodeRegistryListsLocalAndPeersWithoutLeakingTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer sekrit" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok", "version": "peer-1", "model_count": 3,
		})
	}))
	defer srv.Close()
	t.Setenv("AGEZT_PEERS", "nodeB="+srv.URL+"|sekrit")
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdNodeRegistry, nil)
	if err != nil {
		t.Fatal(err)
	}
	nodes, _ := res["nodes"].([]any)
	if len(nodes) != 2 {
		t.Fatalf("nodes = %v, want local + peer", nodes)
	}
	peerNode := findNode(nodes, "nodeB")
	if peerNode == nil {
		t.Fatalf("peer node missing: %v", nodes)
	}
	if peerNode["reachable"] != true || peerNode["version"] != "peer-1" || peerNode["auth"] != "token" {
		t.Fatalf("peer node = %v, want reachable token-auth peer", peerNode)
	}
	if _, leaked := peerNode["token"]; leaked {
		t.Fatalf("node registry leaked token: %v", peerNode)
	}
}

func findNode(nodes []any, name string) map[string]any {
	for _, raw := range nodes {
		row, _ := raw.(map[string]any)
		if row["name"] == name {
			return row
		}
	}
	return nil
}
