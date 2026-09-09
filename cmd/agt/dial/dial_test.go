// SPDX-License-Identifier: MIT

package dial_test

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/cmd/agt/dial"
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

// A recorded address that nothing is listening on (a stale socket
// left by a crashed daemon) returns nil with an actionable
// "(re)start" hint, rather than surfacing a cryptic "connection
// refused" on each command's own call (M239).
func TestNewAtBase_StaleSocketGivesActionableHint(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	base := t.TempDir()
	writeRuntime(t, base, addr)
	t.Setenv("AGEZT_TOKEN", "")

	var errb bytes.Buffer
	if c := dial.NewAtBase(base, &errb); c != nil {
		t.Fatal("NewAtBase should return nil for an unreachable (stale) daemon")
	}
	out := errb.String()
	if !strings.Contains(out, "not responding") {
		t.Errorf("expected a stale-socket hint, got: %q", out)
	}
	if !strings.Contains(out, "start the daemon") && !strings.Contains(out, "(re)start the daemon") {
		t.Errorf("expected a (re)start hint, got: %q", out)
	}
}

// No recorded address (the daemon was never started) returns
// nil with the start hint.
func TestNewAtBase_NeverStartedGivesStartHint(t *testing.T) {
	base := t.TempDir() // no runtime/ files
	t.Setenv("AGEZT_TOKEN", "")
	var errb bytes.Buffer
	if c := dial.NewAtBase(base, &errb); c != nil {
		t.Fatal("NewAtBase should return nil when no daemon is recorded")
	}
	if !strings.Contains(errb.String(), "start the daemon") {
		t.Errorf("expected the start-the-daemon hint, got: %q", errb.String())
	}
}
