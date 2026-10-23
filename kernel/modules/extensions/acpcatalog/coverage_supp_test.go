// SPDX-License-Identifier: MIT

package acpcatalog

import (
	"testing"
)

func TestVersionArgs_Default(t *testing.T) {
	a := Agent{}
	got := a.versionArgs()
	if len(got) != 1 || got[0] != "--version" {
		t.Fatalf("versionArgs() = %v, want [--version]", got)
	}
}

func TestVersionArgs_Custom(t *testing.T) {
	a := Agent{VersionArgs: []string{"-v"}}
	got := a.versionArgs()
	if len(got) != 1 || got[0] != "-v" {
		t.Fatalf("versionArgs() = %v, want [-v]", got)
	}
}

func TestClip_Short(t *testing.T) {
	if clip("hello", 10) != "hello" {
		t.Error("clip should return full string when shorter than max")
	}
}

func TestClip_Long(t *testing.T) {
	got := clip("hello world", 5)
	if got != "hello…" {
		t.Errorf("clip = %q, want hello…", got)
	}
}

func TestCommandMatchesAgent_Exact(t *testing.T) {
	a := Agent{Bin: "gemini"}
	if !commandMatchesAgent("gemini --experimental-acp", a) {
		t.Error("command should match binary name")
	}
}

func TestCommandMatchesAgent_Path(t *testing.T) {
	a := Agent{Bin: "gemini"}
	if !commandMatchesAgent("/usr/local/bin/gemini --experimental-acp", a) {
		t.Error("command with full path should match binary name")
	}
}

func TestCommandMatchesAgent_WindowsExe(t *testing.T) {
	a := Agent{Bin: "codex"}
	if !commandMatchesAgent("C:\\tools\\codex.exe acp", a) {
		t.Error("command with .exe should match")
	}
}

func TestCommandMatchesAgent_Empty(t *testing.T) {
	a := Agent{Bin: "gemini"}
	if commandMatchesAgent("", a) {
		t.Error("empty command should not match")
	}
}

func TestCommandMatchesAgent_Different(t *testing.T) {
	a := Agent{Bin: "gemini"}
	if commandMatchesAgent("codex acp", a) {
		t.Error("unrelated command should not match")
	}
}

func TestResolveCommand_EmptyRefWithFallback(t *testing.T) {
	cmd, ok := ResolveCommand("", "/usr/bin/gemini --acp")
	if !ok || cmd != "/usr/bin/gemini --acp" {
		t.Fatalf("ResolveCommand('', fallback) = (%q, %v), want fallback", cmd, ok)
	}
}

func TestResolveCommand_EmptyRefNoFallback(t *testing.T) {
	cmd, ok := ResolveCommand("", "")
	if ok || cmd != "" {
		t.Fatalf("ResolveCommand('', '') = (%q, %v), want empty", cmd, ok)
	}
}

func TestResolveCommand_UnknownSlug(t *testing.T) {
	cmd, ok := ResolveCommand("bogus-agent", "")
	if ok {
		t.Fatalf("ResolveCommand('bogus-agent') = (%q, %v), want empty", cmd, ok)
	}
}

func TestResolveCommand_KnownButNotInstalled(t *testing.T) {
	// gemini is in the catalog but probably not on PATH in test env.
	cmd, ok := ResolveCommand("gemini", "")
	if ok {
		t.Logf("gemini is installed on this host, skipping test")
		_ = cmd
	}
}
