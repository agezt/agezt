// SPDX-License-Identifier: MIT

package plugin

// White-box test for resolvePluginPath (M422): a bare-name plugin path must be
// resolved to the same absolute file exec will run, so the pin hash and the executed
// binary cannot diverge (pin guarding the wrong binary). Portable across the Windows
// dev box and Linux CI.

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolvePluginPath(t *testing.T) {
	// A path with a separator is returned unchanged (os.Open and exec already agree).
	abs := filepath.Join(t.TempDir(), "plug")
	if got := resolvePluginPath(abs); got != abs {
		t.Errorf("separator path changed: %q -> %q", abs, got)
	}

	// A bare name not on $PATH is returned unchanged (fails closed later).
	const missing = "agezt-no-such-plugin-binary-xyz"
	if got := resolvePluginPath(missing); got != missing {
		t.Errorf("unresolvable bare name changed: %q -> %q", missing, got)
	}

	// A bare name that IS on $PATH resolves to its absolute location — the fix.
	dir := t.TempDir()
	name := "agezt-test-plug"
	if runtime.GOOS == "windows" {
		name += ".bat" // LookPath needs a PATHEXT extension on Windows
	}
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	got := resolvePluginPath(name)
	if got == name {
		t.Fatalf("bare name on PATH was NOT resolved to an absolute path (still %q) — pin could guard the wrong binary", got)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("resolved path is not absolute: %q", got)
	}
}
