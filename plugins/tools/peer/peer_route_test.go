// SPDX-License-Identifier: MIT

package peer

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeList returns a lister backed by a static name->models map; any peer named in
// errPeers returns a discovery error (simulating an unreachable node). It also
// counts calls via the optional calls pointer.
func fakeList(models map[string][]string, calls *int, errPeers ...string) lister {
	errset := map[string]bool{}
	for _, n := range errPeers {
		errset[n] = true
	}
	return func(_ context.Context, p Peer) ([]string, error) {
		if calls != nil {
			*calls++
		}
		if errset[p.Name] {
			return nil, errors.New("unreachable")
		}
		return models[p.Name], nil
	}
}

func twoPeers() map[string]Peer {
	return map[string]Peer{
		"alpha": {Name: "alpha", URL: "http://alpha:1"},
		"bravo": {Name: "bravo", URL: "http://bravo:2"},
	}
}

// TestRemoteRun_AutoRoutesByModel: with no peer named but a model requested, the
// tool discovers which peer serves it and dispatches there.
func TestRemoteRun_AutoRoutesByModel(t *testing.T) {
	var endpoint, body string
	tool := &Tool{
		Peers: twoPeers(),
		post:  fakePost(200, `{"status":"completed","answer":"ok","correlation_id":"c","model":"opus"}`, &endpoint, nil, &body),
		list:  fakeList(map[string][]string{"alpha": {"haiku"}, "bravo": {"opus", "sonnet"}}, nil),
	}
	out, isErr := invoke(t, tool, map[string]string{"task": "do it", "model": "opus"})
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}
	if endpoint != "http://bravo:2/api/v1/runs" {
		t.Errorf("routed to wrong peer: endpoint=%q", endpoint)
	}
	if !strings.Contains(body, `"model":"opus"`) {
		t.Errorf("model not forwarded: %s", body)
	}
}

// TestRemoteRun_AutoRouteNoPeerServesModel: a model no peer serves is an error that
// names what was checked.
func TestRemoteRun_AutoRouteNoPeerServesModel(t *testing.T) {
	tool := &Tool{
		Peers: twoPeers(),
		post:  fakePost(200, `{}`, nil, nil, nil),
		list:  fakeList(map[string][]string{"alpha": {"haiku"}, "bravo": {"opus"}}, nil),
	}
	out, isErr := invoke(t, tool, map[string]string{"task": "x", "model": "gpt-4o"})
	if !isErr {
		t.Fatalf("expected error, got: %s", out)
	}
	if !strings.Contains(out, "no configured peer serves model") || !strings.Contains(out, "gpt-4o") {
		t.Errorf("error message = %q", out)
	}
}

// TestRemoteRun_AutoRouteDeterministic: when several peers serve the model, the
// sorted-first peer is chosen.
func TestRemoteRun_AutoRouteDeterministic(t *testing.T) {
	var endpoint string
	tool := &Tool{
		Peers: twoPeers(),
		post:  fakePost(200, `{"status":"completed","answer":"ok","correlation_id":"c"}`, &endpoint, nil, nil),
		list:  fakeList(map[string][]string{"alpha": {"opus"}, "bravo": {"opus"}}, nil),
	}
	if _, isErr := invoke(t, tool, map[string]string{"task": "x", "model": "opus"}); isErr {
		t.Fatal("unexpected error")
	}
	if endpoint != "http://alpha:1/api/v1/runs" {
		t.Errorf("should pick sorted-first peer 'alpha', got endpoint=%q", endpoint)
	}
}

// TestRemoteRun_AutoRouteSkipsUnreachable: a peer that can't be discovered is skipped
// rather than aborting the search.
func TestRemoteRun_AutoRouteSkipsUnreachable(t *testing.T) {
	var endpoint string
	tool := &Tool{
		Peers: twoPeers(),
		post:  fakePost(200, `{"status":"completed","answer":"ok","correlation_id":"c"}`, &endpoint, nil, nil),
		// alpha (sorted first) is unreachable; bravo serves opus.
		list: fakeList(map[string][]string{"bravo": {"opus"}}, nil, "alpha"),
	}
	if _, isErr := invoke(t, tool, map[string]string{"task": "x", "model": "opus"}); isErr {
		t.Fatal("unexpected error")
	}
	if endpoint != "http://bravo:2/api/v1/runs" {
		t.Errorf("should skip unreachable alpha and route to bravo, got endpoint=%q", endpoint)
	}
}

// TestRemoteRun_NamedPeerSkipsDiscovery: naming a peer bypasses model discovery
// entirely (no lister call), preserving the explicit-dispatch path.
func TestRemoteRun_NamedPeerSkipsDiscovery(t *testing.T) {
	var endpoint string
	calls := 0
	tool := &Tool{
		Peers: twoPeers(),
		post:  fakePost(200, `{"status":"completed","answer":"ok","correlation_id":"c"}`, &endpoint, nil, nil),
		list:  fakeList(map[string][]string{"alpha": {"opus"}, "bravo": {"opus"}}, &calls),
	}
	if _, isErr := invoke(t, tool, map[string]string{"peer": "bravo", "task": "x", "model": "opus"}); isErr {
		t.Fatal("unexpected error")
	}
	if calls != 0 {
		t.Errorf("named peer must skip discovery, but lister was called %d times", calls)
	}
	if endpoint != "http://bravo:2/api/v1/runs" {
		t.Errorf("named dispatch went to %q", endpoint)
	}
}
