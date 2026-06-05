// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderSkillLine_ShadowProgress(t *testing.T) {
	// A shadow skill with evaluation evidence shows "shadow wins/evals" (M402).
	shadow := map[string]any{
		"id": "abc123def456789", "status": "shadow", "name": "diagnose-ci",
		"description": "diagnose a red build",
		"metrics":     map[string]any{"shadow_evals": float64(3), "shadow_wins": float64(2)},
	}
	line := renderSkillLine(shadow)
	if !strings.Contains(line, "[shadow]") || !strings.Contains(line, "shadow 2/3") {
		t.Errorf("shadow line should show progress 'shadow 2/3', got %q", line)
	}
	// An active skill (or a shadow with no evals) shows no shadow progress.
	active := map[string]any{
		"id": "x", "status": "active", "name": "live",
		"metrics": map[string]any{"shadow_evals": float64(0)},
	}
	if strings.Contains(renderSkillLine(active), "shadow ") {
		t.Errorf("active skill must not show shadow progress, got %q", renderSkillLine(active))
	}
	freshShadow := map[string]any{
		"id": "y", "status": "shadow", "name": "new",
		"metrics": map[string]any{"shadow_evals": float64(0)},
	}
	if strings.Contains(renderSkillLine(freshShadow), "shadow ") {
		t.Errorf("a shadow skill with no evals yet must not show progress, got %q", renderSkillLine(freshShadow))
	}
}

func TestCmdSkill_Help(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdSkill([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "skill <subcommand>") {
		t.Errorf("help missing usage; got %q", out.String())
	}
}

func TestCmdSkill_NoSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdSkill(nil, &out, &errOut); code != 2 {
		t.Errorf("missing subcommand should be exit 2, got %d", code)
	}
}

func TestCmdSkill_UnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdSkill([]string{"frobnicate"}, &out, &errOut); code != 2 {
		t.Errorf("unknown subcommand should be exit 2, got %d", code)
	}
}

func TestCmdSkillPromote_RequiresID(t *testing.T) {
	var out, errOut bytes.Buffer
	// No id → usage error before any daemon dial.
	if code := cmdSkill([]string{"promote"}, &out, &errOut); code != 2 {
		t.Errorf("promote without id should be exit 2, got %d", code)
	}
}

func TestCmdSkillShow_RequiresID(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdSkill([]string{"show"}, &out, &errOut); code != 2 {
		t.Errorf("show without id should be exit 2, got %d", code)
	}
}
