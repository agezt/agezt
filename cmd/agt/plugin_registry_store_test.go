// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The installed plugin binary is a file the daemon EXECUTES. A half-written
// binary that the daemon picks up is the worst case in this whole area, so
// the write goes through the platform's atomic writer: a unique temp, fsynced,
// chmodded, renamed into place. The binary appears complete and executable
// or it does not appear at all.
//
// The GATE holds the writer (the site is a raw-write violation if it goes
// back to os.WriteFile). The tests below hold the writer's two promises.

// TestTheInstalledPluginBinaryIsExecutable: the file the daemon runs carries
// the executable bit.
//
// os.WriteFile does not change the mode of an EXISTING file — its perm
// argument only applies at creation. So a re-install over a file whose mode
// had drifted (a tarball extraction, a manual chmod) left the binary
// non-executable and the daemon could not run it. The atomic writer chmods
// the temp before the rename, so the mode is always the one it promises.
//
// The mode bits do not exist on Windows (Go maps only the read-only bit), so
// the assertion is POSIX-only.
func TestTheInstalledPluginBinaryIsExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits do not apply on Windows")
	}
	payload := []byte("the real plugin binary")
	dir, _, file := writeDirRegistry(t, payload, "")
	installDir := t.TempDir()

	// Install once, then drift the mode the way an extraction would.
	if code, _, errs := runReg(t, dir, "--install", "demo", "--dir", installDir); code != 0 {
		t.Fatalf("install exit=%d stderr=%s", code, errs)
	}
	dest := filepath.Join(installDir, file)
	if err := os.Chmod(dest, 0o644); err != nil {
		t.Fatal(err)
	}

	// Re-install over the drifted file.
	if code, _, errs := runReg(t, dir, "--install", "demo", "--dir", installDir); code != 0 {
		t.Fatalf("re-install exit=%d stderr=%s", code, errs)
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o755 {
		t.Fatalf("the installed binary's mode is %o, want 755", perm)
	}
}

// TestTheInstalledPluginBinaryLeavesNoTempFileBehind: the atomic writer's
// promise — the install directory holds the binary and nothing else.
func TestTheInstalledPluginBinaryLeavesNoTempFileBehind(t *testing.T) {
	payload := []byte("the real plugin binary")
	dir, _, file := writeDirRegistry(t, payload, "")
	installDir := t.TempDir()

	if code, _, errs := runReg(t, dir, "--install", "demo", "--dir", installDir); code != 0 {
		t.Fatalf("install exit=%d stderr=%s", code, errs)
	}

	entries, err := os.ReadDir(installDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != file {
		t.Fatalf("the install dir holds %v, want exactly [%s]", names, file)
	}
}
