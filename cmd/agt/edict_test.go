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

// TestCmdEdict_HelpDocsLevel — the dispatcher's help must mention
// `level` now that runtime trust-level changes exist.
func TestCmdEdict_HelpDocsLevel(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdEdict([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "level") {
		t.Errorf("--help missing `level`; got %q", out.String())
	}
}

// TestCmdEdictLevel_RequiresBothArgs — `level` needs capability AND
// level; missing either is a usage error surfaced before any daemon dial.
func TestCmdEdictLevel_RequiresBothArgs(t *testing.T) {
	for _, args := range [][]string{nil, {"shell"}} {
		var out, errOut bytes.Buffer
		if code := cmdEdictLevel(args, &out, &errOut); code != 2 {
			t.Errorf("args=%v exit=%d want 2", args, code)
		}
		if !strings.Contains(errOut.String(), "required") {
			t.Errorf("args=%v stderr missing required note; got %q", args, errOut.String())
		}
	}
}

// TestCmdEdictLevel_HelpDocsLevels — help must teach the level vocabulary.
func TestCmdEdictLevel_HelpDocsLevels(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdEdictLevel([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "L0..L4") {
		t.Errorf("--help missing level vocabulary; got %q", out.String())
	}
}

// TestCmdEdict_HelpDocsMode — the dispatcher's help must mention `mode`.
func TestCmdEdict_HelpDocsMode(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdEdict([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "mode") {
		t.Errorf("--help missing `mode`; got %q", out.String())
	}
}

// TestCmdEdictMode_RequiresMode — `mode` with no arg is a usage error
// surfaced before any daemon dial.
func TestCmdEdictMode_RequiresMode(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdEdictMode(nil, &out, &errOut); code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "mode required") {
		t.Errorf("stderr missing mode-required note; got %q", errOut.String())
	}
}

// TestCmdEdictMode_HelpDocsModes — help must teach allow/deny/prompt.
func TestCmdEdictMode_HelpDocsModes(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdEdictMode([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	s := out.String()
	for _, want := range []string{"allow", "deny", "prompt"} {
		if !strings.Contains(s, want) {
			t.Errorf("--help missing %q; got %q", want, s)
		}
	}
}

// TestExtractTenantFlag covers both --tenant forms and the no-flag case,
// and that the remaining args are preserved in order.
func TestExtractTenantFlag(t *testing.T) {
	cases := []struct {
		in         []string
		wantTenant string
		wantRest   []string
	}{
		{[]string{"add", "rule"}, "", []string{"add", "rule"}},
		{[]string{"--tenant", "acme", "rule"}, "acme", []string{"rule"}},
		{[]string{"rule", "--tenant", "acme"}, "acme", []string{"rule"}},
		{[]string{"--tenant=acme", "rule"}, "acme", []string{"rule"}},
		{[]string{"--tenant"}, "", nil}, // trailing flag, no value
	}
	for _, tc := range cases {
		gotT, gotR := extractTenantFlag(tc.in)
		if gotT != tc.wantTenant {
			t.Errorf("extractTenantFlag(%v) tenant=%q want %q", tc.in, gotT, tc.wantTenant)
		}
		if strings.Join(gotR, ",") != strings.Join(tc.wantRest, ",") {
			t.Errorf("extractTenantFlag(%v) rest=%v want %v", tc.in, gotR, tc.wantRest)
		}
	}
}

// TestWithTenant — non-empty tenant is injected; empty leaves the map alone.
func TestWithTenant(t *testing.T) {
	if m := withTenant("", nil); m != nil {
		t.Errorf("empty tenant should pass nil through; got %v", m)
	}
	m := withTenant("acme", map[string]any{"rule": "x"})
	if m["tenant"] != "acme" || m["rule"] != "x" {
		t.Errorf("withTenant lost data: %v", m)
	}
	if m := withTenant("acme", nil); m["tenant"] != "acme" {
		t.Errorf("withTenant should create a map for a nil input: %v", m)
	}
}

// TestCmdEdictShow_HelpDocsTenant — help advertises --tenant.
func TestCmdEdictShow_HelpDocsTenant(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdEdictShow([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "--tenant") {
		t.Errorf("show --help missing --tenant; got %q", out.String())
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
