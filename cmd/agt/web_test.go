// SPDX-License-Identifier: MIT

package main

// `agt web password` (M933): the CLI set/clear/status path for the console
// password — the recovery story when you can't (or won't) use the Web UI.
// These tests exercise the OFFLINE path (no daemon): the secret lands in the
// vault (encrypted at rest when the machine key is available, M934) and is
// readable back through the same store.

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/creds"
)

// withTempHome points the CLI at a fresh AGEZT_HOME so no test touches a real
// vault or a live daemon's runtime files.
func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("AGEZT_HOME", home)
	return home
}

func TestWebPassword_SetClearStatus_Offline(t *testing.T) {
	home := withTempHome(t)
	var out, errOut bytes.Buffer

	// status before anything: not set.
	if code := cmdWebPassword([]string{"status"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("status: code=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "not set") {
		t.Fatalf("status output = %q, want 'not set'", out.String())
	}

	// set via the prompt path (value + matching confirmation on stdin).
	out.Reset()
	if code := cmdWebPassword([]string{"set"}, strings.NewReader("hunter2\nhunter2\n"), &out, &errOut); code != 0 {
		t.Fatalf("set: code=%d stderr=%s", code, errOut.String())
	}
	if strings.Contains(out.String(), "hunter2") {
		t.Fatal("password value echoed into output")
	}

	store := creds.NewStore(home)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if got := store.Get("AGEZT_WEB_PASSWORD"); got != "hunter2" {
		t.Fatalf("vault value = %q, want hunter2", got)
	}

	// status now reports SET (never the value).
	out.Reset()
	if code := cmdWebPassword([]string{"status"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("status: code=%d", code)
	}
	if !strings.Contains(out.String(), "SET") || strings.Contains(out.String(), "hunter2") {
		t.Fatalf("status output = %q, want SET and no value", out.String())
	}

	// clear removes it.
	out.Reset()
	if code := cmdWebPassword([]string{"clear"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("clear: code=%d stderr=%s", code, errOut.String())
	}
	store2 := creds.NewStore(home)
	if err := store2.Load(); err != nil {
		t.Fatal(err)
	}
	if store2.Has("AGEZT_WEB_PASSWORD") {
		t.Fatal("password still in the vault after clear")
	}
}

func TestWebPassword_PromptMismatchChangesNothing(t *testing.T) {
	home := withTempHome(t)
	var out, errOut bytes.Buffer
	if code := cmdWebPassword([]string{"set"}, strings.NewReader("one\ntwo\n"), &out, &errOut); code == 0 {
		t.Fatal("mismatched confirmation should fail")
	}
	if !strings.Contains(errOut.String(), "do not match") {
		t.Fatalf("stderr = %q, want a mismatch message", errOut.String())
	}
	store := creds.NewStore(home)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if store.Has("AGEZT_WEB_PASSWORD") {
		t.Fatal("vault was written despite the mismatch")
	}
}

func TestWebPassword_EmptyRejected(t *testing.T) {
	withTempHome(t)
	var out, errOut bytes.Buffer
	if code := cmdWebPassword([]string{"set"}, strings.NewReader("\n\n"), &out, &errOut); code == 0 {
		t.Fatal("empty password should be rejected")
	}
	if !strings.Contains(errOut.String(), "clear") {
		t.Fatalf("stderr = %q, want a hint at `web password clear`", errOut.String())
	}
}

func TestWebPassword_SetViaArg(t *testing.T) {
	home := withTempHome(t)
	var out bytes.Buffer
	if code := cmdWebPassword([]string{"set", "from-arg-7"}, strings.NewReader(""), &out, io.Discard); code != 0 {
		t.Fatalf("set via arg: code=%d", code)
	}
	store := creds.NewStore(home)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if got := store.Get("AGEZT_WEB_PASSWORD"); got != "from-arg-7" {
		t.Fatalf("vault value = %q", got)
	}
}
