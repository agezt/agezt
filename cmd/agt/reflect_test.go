// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCmdReflect_Help(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdReflect([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "reflect <subcommand>") {
		t.Errorf("help missing usage; got %q", out.String())
	}
}

func TestCmdReflect_NoSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdReflect(nil, &out, &errOut); code != 2 {
		t.Errorf("missing subcommand should be exit 2, got %d", code)
	}
}

func TestCmdReflect_UnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdReflect([]string{"frobnicate"}, &out, &errOut); code != 2 {
		t.Errorf("unknown subcommand should be exit 2, got %d", code)
	}
}

func TestCmdReflect_RejectsBadFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdReflect([]string{"run", "--nope"}, &out, &errOut); code != 2 {
		t.Errorf("unknown flag should be exit 2, got %d", code)
	}
}
