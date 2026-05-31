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

// TestCmdEdict_HelpDocsDeny — the dispatcher's help must mention
// `deny` now that runtime rule management exists.
func TestCmdEdict_HelpDocsDeny(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdEdict([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "deny") {
		t.Errorf("--help missing `deny`; got %q", out.String())
	}
}

// TestCmdEdictDeny_NoArgsRequiresSubcommand — `agt edict deny`
// with no subcommand must prompt, not connect to the daemon.
func TestCmdEdictDeny_NoArgsRequiresSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdEdictDeny(nil, &out, &errOut); code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "subcommand required") {
		t.Errorf("stderr missing subcommand-required note; got %q", errOut.String())
	}
}

// TestCmdEdictDeny_RejectsUnknownSubcommand pins typo discoverability.
func TestCmdEdictDeny_RejectsUnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdEdictDeny([]string{"als"}, &out, &errOut); code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown subcommand") {
		t.Errorf("stderr should flag unknown subcommand; got %q", errOut.String())
	}
}

// TestCmdEdictDenyAdd_RequiresRule — add with no rule arg is a usage
// error (exit 2), surfaced before any daemon dial.
func TestCmdEdictDenyAdd_RequiresRule(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdEdictDenyAdd(nil, &out, &errOut); code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "rule required") {
		t.Errorf("stderr missing rule-required note; got %q", errOut.String())
	}
}

// TestCmdEdictDenyRemove_RequiresName — rm with no name is a usage error.
func TestCmdEdictDenyRemove_RequiresName(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdEdictDenyRemove(nil, &out, &errOut); code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "name required") {
		t.Errorf("stderr missing name-required note; got %q", errOut.String())
	}
}

// TestCmdEdictDeny_HelpDocsSubcommands — `deny --help` lists list/add/rm.
func TestCmdEdictDeny_HelpDocsSubcommands(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdEdictDeny([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	s := out.String()
	for _, want := range []string{"list", "add", "rm"} {
		if !strings.Contains(s, want) {
			t.Errorf("--help missing %q; got %q", want, s)
		}
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
