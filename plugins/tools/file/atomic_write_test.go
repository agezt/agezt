// SPDX-License-Identifier: MIT

package file

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The file tool writes into the agent's workspace: a `replace` rewrites a file
// in place, and the bytes it writes are the bytes the agent believes are
// there. A write that truncates in place leaves a window in which the file is
// short — and the file tool's `replace` op is the LOW-CLOBBER one, the one an
// agent reaches for when it must not lose the user's content.
//
// So every write the plugin makes goes through the platform's ONE canonical
// writer (internal/atomicfile): a unique temp, fsynced, chmodded, renamed into
// place. The plugin used to keep its own copy of that writer — a second
// implementation of the same mechanism, which is exactly the divergence the
// canonical writer exists to kill.
//
// The GATE holds this at PACKAGE granularity (any raw write in the package is
// a violation). The test below holds it at FILE granularity, so a package that
// stays under the count cannot hide a new raw write in a file the count no
// longer names.
//
// os.CreateTemp is in the set because a package that creates its OWN temp file
// is hand-rolling the atomic writer — the very thing the rule exists to
// prevent.
func TestTheFileToolHasNoRawWrites(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := string(raw)
		n := strings.Count(src, "os.WriteFile(") +
			strings.Count(src, "os.Create(") +
			strings.Count(src, "os.CreateTemp(")
		if n > 0 {
			t.Errorf("%s has %d raw write(s) — route it through the atomic writer", name, n)
		}
	}
}

// TestTheReplaceOpLeavesNoTempFile: the plugin's write goes through the
// canonical writer, so a completed `replace` leaves exactly the file it wrote
// — no scratch file beside it.
//
// The temp is created in the TARGET's directory, so a leak would be visible
// right where the agent is looking.
func TestTheReplaceOpLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	tool, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("alpha beta"), 0o644); err != nil {
		t.Fatal(err)
	}

	invoke(t, tool, fileInput{Op: "replace", Path: "notes.txt", Find: "beta", Replacement: "gamma"})

	got, err := os.ReadFile(filepath.Join(dir, "notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "alpha gamma" {
		t.Fatalf("content = %q, want %q", got, "alpha gamma")
	}

	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.Name() != "notes.txt" {
			t.Errorf("the write left %q beside the target", e.Name())
		}
	}
}
