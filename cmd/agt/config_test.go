// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestCmdConfig_HelpExitsCleanly — pure flag parsing, no daemon.
func TestCmdConfig_HelpExitsCleanly(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdConfig([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "config show") {
		t.Errorf("--help missing 'config show'; got %q", out.String())
	}
}

// TestCmdConfig_RequiresSubcommand — bare `agt config` must
// surface the requirement rather than silently dialing.
func TestCmdConfig_RequiresSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdConfig(nil, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "subcommand required") {
		t.Errorf("stderr should explain requirement; got %q", errOut.String())
	}
}

// TestCmdConfig_RejectsUnknownSubcommand — typo guard.
func TestCmdConfig_RejectsUnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdConfig([]string{"sho"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown subcommand") {
		t.Errorf("stderr should reject unknown subcommand; got %q", errOut.String())
	}
}

// TestCmdConfigShow_HelpExitsCleanly — show --help.
func TestCmdConfigShow_HelpExitsCleanly(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdConfigShow([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	for _, want := range []string{"--json", "paths"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--help missing %q; got %q", want, out.String())
		}
	}
}

// TestCmdConfigShow_RejectsExtraArg — guard against silent
// argument drops like `agt config show extra`.
func TestCmdConfigShow_RejectsExtraArg(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdConfigShow([]string{"unexpected"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unexpected arg") {
		t.Errorf("stderr should reject extra arg; got %q", errOut.String())
	}
}
