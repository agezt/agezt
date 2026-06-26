// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderStandingLine(t *testing.T) {
	o := map[string]any{
		"id": "01ABCDEF2345678", "name": "portfolio watch", "enabled": true,
		"triggers":   []any{map[string]any{"type": "cron"}},
		"initiative": map[string]any{"mode": "act_or_ask"},
	}
	line := renderStandingLine(o)
	for _, want := range []string{"01ABCDEF2345", "[on]", "portfolio watch", "1 trigger", "act_or_ask"} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q missing %q", line, want)
		}
	}
	o["target_status"] = "blocked"
	o["target_error"] = "standing agent ops is retired"
	if line := renderStandingLine(o); !strings.Contains(line, "target:blocked (standing agent ops is retired)") {
		t.Errorf("blocked target should show in line, got %q", line)
	}
	delete(o, "target_error")
	o["target_status"] = "ready"
	if line := renderStandingLine(o); !strings.Contains(line, "target:ready") {
		t.Errorf("ready target should show in line, got %q", line)
	}
	// A disabled order shows [off].
	o["enabled"] = false
	if !strings.Contains(renderStandingLine(o), "[off]") {
		t.Errorf("disabled order should show [off], got %q", renderStandingLine(o))
	}
}

func TestStandingUsageDescribesWakeRules(t *testing.T) {
	var out bytes.Buffer
	standingUsage(&out)
	text := out.String()
	if !strings.Contains(text, "show standing wake rules") {
		t.Fatalf("usage should describe standing orders as wake rules, got %q", text)
	}
	if !strings.Contains(text, "edit <id>") || !strings.Contains(text, "--agent SLUG") {
		t.Fatalf("usage should expose standing edit agent binding, got %q", text)
	}
}

func TestCmdStandingAdd_RequiresNameAndTrigger(t *testing.T) {
	var out, errOut bytes.Buffer
	// No args → usage error (exit 2).
	if code := cmdStandingAdd([]string{"--name", "x"}, &out, &errOut); code != 2 {
		t.Errorf("add without a trigger should exit 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "required") {
		t.Errorf("expected a 'required' message, got %q", errOut.String())
	}
}

func TestCmdStandingAdd_RejectsBadCooldown(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdStandingAdd([]string{"--name", "x", "--event", "task.failed", "--cooldown", "soon"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("bad cooldown should exit 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--cooldown") {
		t.Fatalf("expected cooldown error, got %q", errOut.String())
	}
}

func TestCmdStandingEdit_RequiresIDAndField(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdStandingEdit(nil, &out, &errOut); code != 2 {
		t.Fatalf("edit without id should exit 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "standing edit <id>") {
		t.Fatalf("expected edit usage, got %q", errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := cmdStandingEdit([]string{"order-1"}, &out, &errOut); code != 2 {
		t.Fatalf("edit without a field should exit 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "at least one field") {
		t.Fatalf("expected field-required error, got %q", errOut.String())
	}
}

func TestCmdStandingEdit_RejectsBadValues(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdStandingEdit([]string{"order-1", "--cooldown", "soon"}, &out, &errOut); code != 2 {
		t.Fatalf("bad cooldown should exit 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--cooldown") {
		t.Fatalf("expected cooldown error, got %q", errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := cmdStandingEdit([]string{"order-1", "--assure", "-1"}, &out, &errOut); code != 2 {
		t.Fatalf("bad assure should exit 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--assure") {
		t.Fatalf("expected assure error, got %q", errOut.String())
	}
}

func TestCmdStanding_UnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdStanding([]string{"frobnicate"}, &out, &errOut); code != 0 {
		// unknown prints usage to stderr and returns the usage code (0)
	}
	if !strings.Contains(errOut.String(), "unknown subcommand") {
		t.Errorf("expected unknown-subcommand message, got %q", errOut.String())
	}
}

func TestCmdStandingSetEnabled_RequiresID(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdStandingSetEnabled(nil, &out, &errOut, false); code != 2 {
		t.Errorf("pause without id should exit 2, got %d", code)
	}
}
