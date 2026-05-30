// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

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
