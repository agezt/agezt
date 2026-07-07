// SPDX-License-Identifier: MIT

package shell

import (
	"runtime"
	"testing"
)

func TestShellCoverageName(t *testing.T) {
	tool := NewWithWarden(nil)
	if got := tool.Name(); got != "shell" {
		t.Fatalf("Name = %q, want %q", got, "shell")
	}
}

func TestShellCoverageResolveShell(t *testing.T) {
	// Explicit Shell + ShellArg override.
	tool := &Tool{Shell: "bash", ShellArg: "-lc"}
	exe, arg := tool.resolveShell()
	if exe != "bash" || arg != "-lc" {
		t.Fatalf("explicit override = (%q, %q)", exe, arg)
	}

	// Shell set but ShellArg empty → defaults to "-c".
	tool = &Tool{Shell: "bash"}
	exe, arg = tool.resolveShell()
	if exe != "bash" || arg != "-c" {
		t.Fatalf("Shell without arg = (%q, %q)", exe, arg)
	}

	// No override → platform default.
	tool = &Tool{}
	exe, arg = tool.resolveShell()
	if runtime.GOOS == "windows" {
		if exe != "cmd" || arg != "/C" {
			t.Fatalf("windows default = (%q, %q)", exe, arg)
		}
	} else {
		if exe != "sh" || arg != "-c" {
			t.Fatalf("unix default = (%q, %q)", exe, arg)
		}
	}
}

func TestShellCoverageShellHint(t *testing.T) {
	// ShellHint delegates to resolveShell; verify via a Tool with an override.
	tool := &Tool{Shell: "zsh"}
	if exe, arg := tool.ShellHint(); exe != "zsh" || arg != "-c" {
		t.Fatalf("ShellHint = (%q, %q)", exe, arg)
	}

	// Without override, returns the platform default.
	tool = &Tool{}
	exe, _ := tool.ShellHint()
	if runtime.GOOS == "windows" {
		if exe != "cmd" {
			t.Fatalf("windows ShellHint = %q", exe)
		}
	} else {
		if exe != "sh" {
			t.Fatalf("unix ShellHint = %q", exe)
		}
	}
}
