// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCmdState_NoSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdState(nil, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "subcommand required") {
		t.Errorf("stderr missing subcommand-required note; got %q", errOut.String())
	}
}

func TestCmdState_UnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdState([]string{"set"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown subcommand") {
		t.Errorf("stderr should flag unknown subcommand; got %q", errOut.String())
	}
}

func TestCmdState_HelpDocsBothSubcommands(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdState([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	for _, want := range []string{"list", "get", "namespace"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--help missing %q; got %q", want, out.String())
		}
	}
}

func TestCmdStateGet_RejectsIncompleteArgs(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdStateGet([]string{"only-ns"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "required") {
		t.Errorf("stderr should explain required args; got %q", errOut.String())
	}
}

func TestCmdStateGet_HelpDocsExitCodes(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdStateGet([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "exit 0 = found") {
		t.Errorf("--help should document exit codes; got %q", out.String())
	}
}
