// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRuntime(t *testing.T, base, addr string) {
	t.Helper()
	rt := filepath.Join(base, "runtime")
	if err := os.MkdirAll(rt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rt, "control.addr"), []byte(addr+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rt, "control.token"), []byte("tok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A recorded address that nothing is listening on (a stale socket left by a
// crashed daemon) returns nil with an actionable "(re)start" hint, rather than
// surfacing a cryptic "connection refused" on each command's own call (M239).
func TestDialBase_StaleSocketGivesActionableHint(t *testing.T) {
	// Grab a real free port, then close it so connects are refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	base := t.TempDir()
	writeRuntime(t, base, addr)
	// NewClient prefers AGEZT_TOKEN; clear it so the on-disk token is used and
	// the path is deterministic.
	t.Setenv("AGEZT_TOKEN", "")

	var errb bytes.Buffer
	if c := dialBase(base, &errb); c != nil {
		t.Fatal("dialBase should return nil for an unreachable (stale) daemon")
	}
	out := errb.String()
	if !strings.Contains(out, "not responding") || !strings.Contains(out, "start the daemon") && !strings.Contains(out, "(re)start the daemon") {
		t.Errorf("expected a stale-socket hint, got: %q", out)
	}
}

// No recorded address (the daemon was never started) returns nil with the
// start hint.
func TestDialBase_NeverStartedGivesStartHint(t *testing.T) {
	base := t.TempDir() // no runtime/ files
	t.Setenv("AGEZT_TOKEN", "")
	var errb bytes.Buffer
	if c := dialBase(base, &errb); c != nil {
		t.Fatal("dialBase should return nil when no daemon is recorded")
	}
	if !strings.Contains(errb.String(), "start the daemon") {
		t.Errorf("expected the start-the-daemon hint, got: %q", errb.String())
	}
}
