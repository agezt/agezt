// SPDX-License-Identifier: MIT

package acpagent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/acp"
)

// TestACPPeerProcess is not a real test: when AGEZT_ACP_PEER=1 it acts as a
// standalone external ACP agent, serving the protocol over stdin/stdout, so the
// live test below can spawn it as a genuine subprocess. It exits the process
// directly so the testing framework never prints a summary onto stdout (which
// would corrupt the JSON-RPC stream the bridge is reading).
func TestACPPeerProcess(t *testing.T) {
	if os.Getenv("AGEZT_ACP_PEER") != "1" {
		return // ordinary run: do nothing
	}
	srv := acp.New(&peerRunner{chunks: []string{"live ", "ACP ", "round-trip ok"}}, os.Stdin, os.Stdout)
	_ = srv.Serve(context.Background())
	os.Exit(0)
}

// TestLive_SpawnRealACPPeer exercises the real spawnAgent path (no injected
// dial): the bridge launches a true subprocess that speaks ACP over its stdio,
// initialises, opens a session, prompts, and relays the streamed answer. This is
// the demo gate proving the client works on the wire end-to-end, not just
// against an in-process fake.
func TestLive_SpawnRealACPPeer(t *testing.T) {
	// Re-exec this test binary as the ACP peer. The temp test-binary path has no
	// spaces, so it can go to the shell unquoted (cmd /C's quote-stripping makes
	// a quoted path unreliable on Windows); skip the test if that assumption ever
	// breaks rather than misfire.
	if strings.ContainsAny(os.Args[0], " \t") {
		t.Skip("test binary path contains spaces; live shell spawn would need per-shell quoting")
	}
	cmd := os.Args[0] + " -test.run=^TestACPPeerProcess$"

	// The spawned child inherits our environment; the flag turns it into a peer.
	t.Setenv("AGEZT_ACP_PEER", "1")

	tool := &Tool{Cmd: cmd, Cwd: ".", dial: spawnAgent} // the REAL dial

	in, _ := json.Marshal(map[string]string{"task": "say something"})
	res, err := tool.Invoke(context.Background(), in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("live ACP bridge errored: %s", res.Output)
	}
	if !strings.Contains(res.Output, "live ACP round-trip ok") {
		t.Errorf("expected the peer's streamed answer, got:\n%s", res.Output)
	}
}
