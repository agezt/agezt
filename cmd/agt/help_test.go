// SPDX-License-Identifier: MIT

package main

// Help-coverage guard (M935). The old flat help rotted: ~20 dispatched
// commands (backup, warden, standing, workflow, …) were simply absent from
// `agt help`, so operators couldn't discover them. These tests tie the
// dispatch table in main.go to the help table in help.go: a new command that
// isn't documented fails CI.

import (
	"bytes"
	"sort"
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

// TestHelp_CoversEveryRegisteredCommand: every real top-level command in the
// registry must have a help entry. The dispatch layer now routes through
// CommandRegistry, so scanning main.go's old switch would miss newly registered
// commands and let `agt <cmd> -h` fall through to live command execution.
func TestHelp_CoversEveryRegisteredCommand(t *testing.T) {
	documented := helpNames()
	var missing []string
	for _, cmd := range AllCommands() {
		if !documented[cmd.Name] {
			missing = append(missing, cmd.Name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("commands registered in CommandRegistry but missing from helpGroups (help.go): %v\n"+
			"add a commandHelp entry so the command is discoverable", missing)
	}
}

func TestHelp_DocumentsOnlyRegisteredCommands(t *testing.T) {
	registered := map[string]bool{}
	for _, cmd := range AllCommands() {
		registered[cmd.Name] = true
	}
	var stale []string
	for name := range helpNames() {
		if !registered[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("helpGroups documents commands that are not registered: %v", stale)
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

// TestUniformDashH (M936): `agt <cmd> -h` answers from the help table for
// EVERY documented command, exits 0, and never reaches the command's own code.
// This is the regression guard for the class of bug where a command treats its
// first arg as data — `agt run -h` used to send "-h" to the live agent as an
// intent and bill a completion for it.
func TestUniformDashH(t *testing.T) {
	// No daemon, fresh home: if interception leaked through to a command that
	// dials the control plane, it would error (non-zero) — failing this test.
	t.Setenv("AGEZT_HOME", t.TempDir())
	for _, cmd := range AllCommands() {
		for _, flag := range []string{"-h", "--help"} {
			var out, errOut bytes.Buffer
			if code := run([]string{cmd.Name, flag}, &out, &errOut); code != 0 {
				t.Errorf("agt %s %s: exit=%d stderr=%s (must answer from the help table)", cmd.Name, flag, code, errOut.String())
				continue
			}
			if !strings.Contains(out.String(), cmd.Name) {
				t.Errorf("agt %s %s output does not mention the command:\n%s", cmd.Name, flag, out.String())
			}
		}
	}
}

// TestUnknownCommand_ShortErrorWithSuggestion: a typo'd top-level command gets
// a one-line error + did-you-mean + a pointer at `agt help` — not an 80-line
// help dump that buries the error.
func TestUnknownCommand_ShortErrorWithSuggestion(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"jurnal"}, &out, &errOut); code != 2 {
		t.Fatalf("exit=%d, want 2", code)
	}
	msg := errOut.String()
	if !strings.Contains(msg, "journal") {
		t.Errorf("no did-you-mean suggestion in: %s", msg)
	}
	if !strings.Contains(msg, "help") {
		t.Errorf("no pointer at `agt help` in: %s", msg)
	}
	if lines := strings.Count(msg, "\n"); lines > 3 {
		t.Errorf("error is %d lines — the old full-help dump is back?", lines)
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
	if code := cmdHelp([]string{"agent"}, &out, &errOut); code != 0 {
		t.Fatalf("help agent: code=%d", code)
	}
	if !strings.Contains(out.String(), "repair-status") || !strings.Contains(out.String(), "wake|repair") {
		t.Errorf("help agent output missing repair/wake lifecycle commands:\n%s", out.String())
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

func TestHelp_AgentAutomationLanguage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdHelp(nil, &out, &errOut); code != 0 {
		t.Fatalf("overview: code=%d stderr=%s", code, errOut.String())
	}
	overview := out.String()
	for _, want := range []string{
		"typed cron/event jobs",
		"durable wake rules for agents",
	} {
		if !strings.Contains(overview, want) {
			t.Fatalf("help overview missing %q:\n%s", want, overview)
		}
	}
	for _, old := range []string{"recurring autonomous intents", "event + cron triggered intents"} {
		if strings.Contains(overview, old) {
			t.Fatalf("help overview still uses old automation wording %q:\n%s", old, overview)
		}
	}

	out.Reset()
	errOut.Reset()
	if code := cmdHelp([]string{"schedule"}, &out, &errOut); code != 0 {
		t.Fatalf("help schedule: code=%d stderr=%s", code, errOut.String())
	}
	detail := out.String()
	if !strings.Contains(detail, "workflow/system-task/tool targets") ||
		!strings.Contains(detail, "schedule add --system-task catalog_sync --every 24h") ||
		!strings.Contains(detail, "sync models.dev/api.json without waking an agent") ||
		!strings.Contains(detail, "--continuous <dur>") ||
		!strings.Contains(detail, "cycle loop") ||
		strings.Contains(detail, "<intent>") {
		t.Fatalf("help schedule should present typed targets, not generic prompt-like intent text:\n%s", detail)
	}
}
