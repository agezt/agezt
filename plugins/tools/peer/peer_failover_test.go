// SPDX-License-Identifier: MIT

package peer

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// postByEndpoint returns a poster that records every endpoint it was called with and
// returns a transport error for endpoints whose host is in deadHosts, otherwise a
// scripted success. This models a peer that is unreachable at the network level (no
// HTTP response), which is the only failure that triggers failover (M206).
func postByEndpoint(deadHosts map[string]bool, okBody string, calls *[]string) poster {
	return func(_ context.Context, endpoint, _ string, _ []byte) (int, []byte, error) {
		if calls != nil {
			*calls = append(*calls, endpoint)
		}
		for host := range deadHosts {
			if strings.Contains(endpoint, host) {
				return 0, nil, errors.New("connection refused")
			}
		}
		return 200, []byte(okBody), nil
	}
}

// TestAutoRoute_FailsOverToNextServer: the primary serving peer is unreachable at the
// transport level, so the task falls back to the next peer that serves the model.
func TestAutoRoute_FailsOverToNextServer(t *testing.T) {
	var calls []string
	tool := &Tool{
		Peers: twoPeers(), // alpha=http://alpha:1, bravo=http://bravo:2
		list:  fakeList(map[string][]string{"alpha": {"opus"}, "bravo": {"opus"}}, nil),
		// alpha is reachable for discovery but its /runs POST fails at transport level.
		post: postByEndpoint(map[string]bool{"alpha:1": true}, `{"status":"completed","answer":"ok","correlation_id":"c","model":"opus"}`, &calls),
	}
	out, isErr := invoke(t, tool, map[string]string{"task": "x", "model": "opus"})
	if isErr {
		t.Fatalf("failover should succeed via bravo, got error: %s", out)
	}
	if !strings.Contains(out, "peer=bravo") {
		t.Errorf("answer should come from bravo: %s", out)
	}
	// It tried alpha first (transport error), then bravo.
	if len(calls) != 2 || !strings.Contains(calls[0], "alpha:1") || !strings.Contains(calls[1], "bravo:2") {
		t.Errorf("expected alpha then bravo, got %v", calls)
	}
}

// TestAutoRoute_AllServersUnreachable: when every serving peer is unreachable, the
// error names how many were tried.
func TestAutoRoute_AllServersUnreachable(t *testing.T) {
	tool := &Tool{
		Peers: twoPeers(),
		list:  fakeList(map[string][]string{"alpha": {"opus"}, "bravo": {"opus"}}, nil),
		post:  postByEndpoint(map[string]bool{"alpha:1": true, "bravo:2": true}, `{}`, nil),
	}
	out, isErr := invoke(t, tool, map[string]string{"task": "x", "model": "opus"})
	if !isErr {
		t.Fatalf("expected error when all peers unreachable, got: %s", out)
	}
	if !strings.Contains(out, "all 2 peers serving") || !strings.Contains(out, "alpha") || !strings.Contains(out, "bravo") {
		t.Errorf("error should name all tried peers: %s", out)
	}
}

// TestAutoRoute_DoesNotFailOverOnRunFailure: a peer that RESPONDS with a failure
// (it accepted and ran the task) must NOT be retried elsewhere — retrying could
// double-execute side effects.
func TestAutoRoute_DoesNotFailOverOnRunFailure(t *testing.T) {
	var calls []string
	tool := &Tool{
		Peers: twoPeers(),
		list:  fakeList(map[string][]string{"alpha": {"opus"}, "bravo": {"opus"}}, nil),
		// alpha responds 502 status:failed (a real response, not a transport error).
		post: func(_ context.Context, endpoint, _ string, _ []byte) (int, []byte, error) {
			calls = append(calls, endpoint)
			if strings.Contains(endpoint, "alpha:1") {
				return 502, []byte(`{"status":"failed","error":"provider exhausted","correlation_id":"run-x"}`), nil
			}
			return 200, []byte(`{"status":"completed","answer":"ok","correlation_id":"c"}`), nil
		},
	}
	out, isErr := invoke(t, tool, map[string]string{"task": "x", "model": "opus"})
	if !isErr {
		t.Fatalf("a peer-run failure must surface, not fail over: %s", out)
	}
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "provider exhausted") {
		t.Errorf("error should surface alpha's failure: %s", out)
	}
	// Only alpha was tried — bravo must NOT have been contacted.
	if len(calls) != 1 || !strings.Contains(calls[0], "alpha:1") {
		t.Errorf("must not fail over past a responding peer, calls=%v", calls)
	}
}

// TestNamedPeer_TransportErrorUnchanged: a single named peer that fails at transport
// level keeps the original single-peer error message (no failover machinery).
func TestNamedPeer_TransportErrorUnchanged(t *testing.T) {
	tool := &Tool{
		Peers: twoPeers(),
		list:  fakeList(map[string][]string{"alpha": {"opus"}, "bravo": {"opus"}}, nil),
		post:  postByEndpoint(map[string]bool{"alpha:1": true}, `{}`, nil),
	}
	out, isErr := invoke(t, tool, map[string]string{"peer": "alpha", "task": "x"})
	if !isErr {
		t.Fatalf("expected transport error, got: %s", out)
	}
	if !strings.Contains(out, "remote_run: POST") || !strings.Contains(out, "failed") {
		t.Errorf("single named peer should keep the original error message: %s", out)
	}
}
