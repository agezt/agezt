// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCmdWorld_Help(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdWorld([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "world <subcommand>") {
		t.Errorf("help missing usage; got %q", out.String())
	}
}

func TestCmdWorld_NoSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdWorld(nil, &out, &errOut); code != 2 {
		t.Errorf("missing subcommand should be exit 2, got %d", code)
	}
}

func TestCmdWorld_UnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdWorld([]string{"frobnicate"}, &out, &errOut); code != 2 {
		t.Errorf("unknown subcommand should be exit 2, got %d", code)
	}
}

func TestCmdWorldAdd_RequiresName(t *testing.T) {
	var out, errOut bytes.Buffer
	// --kind with no positional name → usage error (no daemon dial attempted).
	if code := cmdWorldAdd([]string{"--kind", "project"}, &out, &errOut); code != 2 {
		t.Errorf("add without name should be exit 2, got %d", code)
	}
}

func TestCmdWorldRelate_NeedsThreeArgs(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdWorldRelate([]string{"Lictor", "depends_on"}, &out, &errOut); code != 2 {
		t.Errorf("relate with 2 args should be exit 2, got %d", code)
	}
}
