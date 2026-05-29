// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestCmdEdict_HelpDocsShow — the dispatcher's help must mention
// `show` so operators learn what's available before reaching
// for the daemon connection.
func TestCmdEdict_HelpDocsShow(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdEdict([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "show") {
		t.Errorf("--help missing `show`; got %q", out.String())
	}
}

// TestCmdEdict_NoArgsRequiresSubcommand prevents `agt edict`
// from silently doing nothing — operators get a clear "subcommand
// required" prompt.
func TestCmdEdict_NoArgsRequiresSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdEdict(nil, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "subcommand required") {
		t.Errorf("stderr missing subcommand-required note; got %q", errOut.String())
	}
}

// TestCmdEdict_RejectsUnknownSubcommand — same discoverability
// pin for typos.
func TestCmdEdict_RejectsUnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdEdict([]string{"sho"}, &out, &errOut) // typo of "show"
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown subcommand") {
		t.Errorf("stderr should flag the unknown subcommand; got %q", errOut.String())
	}
}

// TestCmdEdictShow_HelpDocsJSON — operators must discover
// --json from the help text.
func TestCmdEdictShow_HelpDocsJSON(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdEdictShow([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "--json") {
		t.Errorf("--help missing --json; got %q", out.String())
	}
}
