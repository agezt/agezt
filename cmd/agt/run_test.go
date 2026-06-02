// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRunIntent_PositionalArgs(t *testing.T) {
	got, err := resolveRunIntent([]string{"summarise", "the", "repo"}, "", strings.NewReader("IGNORED"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "summarise the repo" {
		t.Errorf("intent = %q, want %q", got, "summarise the repo")
	}
}

func TestResolveRunIntent_Stdin(t *testing.T) {
	// The sole positional "-" reads all of stdin.
	got, err := resolveRunIntent([]string{"-"}, "", strings.NewReader("  a multi\nline prompt\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "a multi\nline prompt" {
		t.Errorf("stdin intent = %q, want the trimmed multi-line text", got)
	}
}

func TestResolveRunIntent_File(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "prompt.txt")
	if err := os.WriteFile(p, []byte("from a file\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// --file takes precedence over positional + stdin.
	got, err := resolveRunIntent([]string{"ignored"}, p, strings.NewReader("ignored too"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "from a file" {
		t.Errorf("file intent = %q, want %q", got, "from a file")
	}
}

func TestResolveRunIntent_MissingFileErrors(t *testing.T) {
	if _, err := resolveRunIntent(nil, filepath.Join(t.TempDir(), "nope.txt"), strings.NewReader("")); err == nil {
		t.Error("a missing --file should return an error")
	}
}

func TestResolveRunIntent_Empty(t *testing.T) {
	got, err := resolveRunIntent(nil, "", strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("empty input should yield empty intent, got %q", got)
	}
}
