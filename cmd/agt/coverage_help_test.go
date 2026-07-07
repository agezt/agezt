// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestExecuteCommandUnknown covers the unknown-command branch of ExecuteCommand,
// including the suggestion path.
func TestExecuteCommandUnknown(t *testing.T) {
	var out, errOut bytes.Buffer
	// A name close to a real command triggers the suggestion branch.
	code := ExecuteCommand("agnt", []string{}, &out, &errOut)
	if code != 2 {
		t.Fatalf("unknown command exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Fatalf("stderr = %q", errOut.String())
	}

	// A name with no close match still returns 2.
	out.Reset()
	errOut.Reset()
	if code := ExecuteCommand("zzzzzzzzz", []string{}, &out, &errOut); code != 2 {
		t.Fatalf("nonsense command exit=%d want 2", code)
	}
}

// TestLookupAndRegister exercises lookup for a known and unknown command.
func TestLookupAndRegister(t *testing.T) {
	if lookup("run") == nil {
		t.Fatal("lookup(run) = nil, want registered command")
	}
	if lookup("definitely-not-a-command") != nil {
		t.Fatal("lookup(unknown) != nil")
	}
}

// TestAllCommandsHelpPaths drives every registered command with -h and --help.
// Help/usage output paths are among the largest sources of uncovered code in
// CLI handlers, and none of them require a running control-plane server.
func TestAllCommandsHelpPaths(t *testing.T) {
	// A temp AGEZT_HOME keeps any accidental file writes contained.
	t.Setenv("AGEZT_HOME", t.TempDir())

	for _, cmd := range AllCommands() {
		cmd := cmd
		t.Run(cmd.Name, func(t *testing.T) {
			for _, flag := range []string{"-h", "--help", "help"} {
				var out, errOut bytes.Buffer
				// We only assert it does not panic; exit code varies by command.
				_ = cmd.Run([]string{flag}, &out, &errOut)
			}
			if cmd.HelpHandler != nil {
				var out, errOut bytes.Buffer
				_ = cmd.HelpHandler([]string{}, &out, &errOut)
			}
		})
	}
}

// TestAllCommandsNoArgs drives every registered command with no arguments.
// Many handlers print usage or a "needs subcommand" error when given no args,
// which exercises their argument-validation branches without a server.
func TestAllCommandsNoArgs(t *testing.T) {
	t.Setenv("AGEZT_HOME", t.TempDir())
	for _, cmd := range AllCommands() {
		cmd := cmd
		t.Run(cmd.Name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("command %q panicked with no args: %v", cmd.Name, r)
				}
			}()
			var out, errOut bytes.Buffer
			_ = cmd.Run(nil, &out, &errOut)
		})
	}
}

// TestAllCommandsUnknownSubcommand drives every registered command with an
// invalid subcommand token, exercising the "unknown subcommand" branches.
func TestAllCommandsUnknownSubcommand(t *testing.T) {
	t.Setenv("AGEZT_HOME", t.TempDir())
	for _, cmd := range AllCommands() {
		cmd := cmd
		t.Run(cmd.Name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("command %q panicked with bogus subcommand: %v", cmd.Name, r)
				}
			}()
			var out, errOut bytes.Buffer
			_ = cmd.Run([]string{"__no_such_subcommand__"}, &out, &errOut)
		})
	}
}
