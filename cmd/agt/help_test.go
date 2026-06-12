// SPDX-License-Identifier: MIT

package main

// Help-coverage guard (M935). The old flat help rotted: ~20 dispatched
// commands (backup, warden, standing, workflow, …) were simply absent from
// `agt help`, so operators couldn't discover them. These tests tie the
// dispatch table in main.go to the help table in help.go: a new command that
// isn't documented fails CI.

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

// helpNames returns the set of command names the help table documents.
func helpNames() map[string]bool {
	out := map[string]bool{}
	for _, g := range helpGroups() {
		for _, c := range g.commands {
			out[c.name] = true
		}
	}
	return out
}

// dispatchAliases are dispatch tokens that are aliases of documented commands
// (flags and synonyms), not commands of their own.
var dispatchAliases = map[string]bool{
	"-v": true, "--version": true,
	"-h": true, "--help": true,
}

var caseRe = regexp.MustCompile(`(?m)^\tcase ("(?:[^"]+)"(?:, "(?:[^"]+)")*):`)

// TestHelp_CoversEveryCommand: every `case "<cmd>"` in main.go's run() switch
// must have a help entry (aliases excepted). One-directional on purpose: the
// help table may document a command a sibling branch is still landing.
func TestHelp_CoversEveryCommand(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	// Scope to the run() dispatch only — main.go has other switches whose
	// cases are subcommands, not top-level commands.
	body := string(src)
	if i := strings.Index(body, "func run("); i >= 0 {
		body = body[i:]
		if j := strings.Index(body[1:], "\nfunc "); j >= 0 {
			body = body[:j+1]
		}
	}
	documented := helpNames()
	var missing []string
	for _, m := range caseRe.FindAllStringSubmatch(body, -1) {
		for _, tok := range strings.Split(m[1], ",") {
			name := strings.Trim(strings.TrimSpace(tok), `"`)
			if dispatchAliases[name] || documented[name] {
				continue
			}
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Errorf("commands dispatched in main.go but missing from helpGroups (help.go): %v\n"+
			"add a commandHelp entry so the command is discoverable", missing)
	}
}

// TestHelp_EntriesAreComplete: every documented command has a non-empty
// summary and at least one detail line, so `agt help <cmd>` never prints an
// empty block.
func TestHelp_EntriesAreComplete(t *testing.T) {
	for _, g := range helpGroups() {
		if strings.TrimSpace(g.title) == "" {
			t.Error("a help group has an empty title")
		}
		for _, c := range g.commands {
			if strings.TrimSpace(c.summary) == "" {
				t.Errorf("%s: empty summary", c.name)
			}
			if len(c.detail) == 0 {
				t.Errorf("%s: no detail lines (agt help %s would be empty)", c.name, c.name)
			}
		}
	}
}

// TestCmdHelp_OverviewAndDetail: the overview lists groups; `help <cmd>`
// prints that command's detail; an unknown command suggests near-misses.
func TestCmdHelp_OverviewAndDetail(t *testing.T) {
	var out, errOut bytes.Buffer

	if code := cmdHelp(nil, &out, &errOut); code != 0 {
		t.Fatalf("overview: code=%d", code)
	}
	for _, want := range []string{"Run & control", "Journal & audit", "run ", "journal", "vault"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("overview missing %q", want)
		}
	}

	out.Reset()
	if code := cmdHelp([]string{"journal"}, &out, &errOut); code != 0 {
		t.Fatalf("help journal: code=%d", code)
	}
	if !strings.Contains(out.String(), "journal export") {
		t.Errorf("help journal output missing the export usage:\n%s", out.String())
	}

	out.Reset()
	errOut.Reset()
	if code := cmdHelp([]string{"jurnal"}, &out, &errOut); code == 0 {
		t.Fatal("unknown command should fail")
	}
	if !strings.Contains(errOut.String(), "journal") {
		t.Errorf("typo should suggest 'journal', got: %s", errOut.String())
	}
}
