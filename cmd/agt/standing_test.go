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
	// A disabled order shows [off].
	o["enabled"] = false
	if !strings.Contains(renderStandingLine(o), "[off]") {
		t.Errorf("disabled order should show [off], got %q", renderStandingLine(o))
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
