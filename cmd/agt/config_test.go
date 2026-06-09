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
	for _, want := range []string{"show", "ls", "get", "set", "schema register", "schema unregister"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--help missing %q; got %q", want, out.String())
		}
	}
}

// The new subcommands must validate their args BEFORE dialing the daemon, so
// these run with no daemon and still exit 2 (usage) rather than hanging or 1.
func TestCmdConfig_SubcommandArgValidation(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
		errc string
	}{
		{"set needs env", []string{"set"}, 2, "ENV required"},
		{"get needs env", []string{"get"}, 2, "ENV required"},
		{"get rejects extra", []string{"get", "AGEZT_MODEL", "extra"}, 2, "unexpected arg"},
		{"schema register needs file", []string{"schema", "register"}, 2, "FILE required"},
		{"schema unregister needs id", []string{"schema", "unregister"}, 2, "ID required"},
		{"schema rejects extra", []string{"schema", "bogus"}, 2, "unexpected arg"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := cmdConfig(c.args, &out, &errOut); code != c.want {
				t.Errorf("exit=%d want %d; stderr=%q", code, c.want, errOut.String())
			}
			if !strings.Contains(errOut.String(), c.errc) {
				t.Errorf("stderr=%q want contains %q", errOut.String(), c.errc)
			}
		})
	}
}

// schema register with a missing file fails (exit 1) at ReadFile, before dialing.
func TestCmdConfigSchemaRegister_MissingFile(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdConfig([]string{"schema", "register", "does-not-exist.json"}, &out, &errOut)
	if code != 1 {
		t.Errorf("exit=%d want 1; stderr=%q", code, errOut.String())
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
