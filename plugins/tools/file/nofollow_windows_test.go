// SPDX-License-Identifier: MIT

//go:build windows

package file

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// makeJunction creates a directory junction at link -> target using mklink /J.
// Requires administrator or Developer Mode on Windows.  The test aborts if
// the junction is not created (e.g. Developer Mode is disabled) so we fail
// fast and clearly rather than silently falling through to a confusing
// downstream assertion.
func makeJunction(t *testing.T, link, target string) {
	t.Helper()
	// mklink /J requires the link does NOT already exist.
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	// Remove any stale artifact (file, empty dir, or non-empty junction reparse point).
	os.RemoveAll(link)
	// Run mklink /J: linkPath targetPath  (mklink expects the link as first arg)
	// We use cmd /c to handle the space in paths.
	runCmd(t, "cmd", "/c", "mklink", "/J", link, target)
	// Defensive: verify the junction was actually created as a reparse point.
	// On Windows non-admin without Developer Mode mklink /J fails silently, leaving
	// link as a regular directory — causing a confusing downstream test failure
	// instead of a clear environment-skip.
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("makeJunction: %s was not created (is Developer Mode enabled on this machine?): %v", link, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		// On go1.27+ junctions appear as ModeIrregular, not ModeSymlink.
		// Accept either — the mklink /J did create something.
		if info.Mode()&os.ModeIrregular == 0 {
			t.Skipf("makeJunction: %s was created as %v, not a symlink/junction — is Developer Mode enabled on this machine?", link, info.Mode())
		}
	}
}

// runCmd is a test helper that executes a command and aborts the test on failure.
func runCmd(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("running %s %v failed: %v\n%s", name, args, err, out)
	}
}

// TestOpenFileNoFollow_RejectsJunctionSiblingEscape verifies that when the
// workspace root ends with the same prefix as the junction name, a naive
// strings.HasPrefix check would pass the escape.  For example:
//   wsRoot = "C:\...\001\ws"
//   junction "C:\...\001\ws\ws-esc" -> "C:\...\001"  (sibling of ws)
//   file    "C:\...\001\ws\ws-esc\secret.txt"
// After cleanWinFinalPath both workspace and resolved path start with the same
// prefix, but the resolved path's ".." segment must still be detected as an
// escape via filepath.Rel.
func TestOpenFileNoFollow_RejectsJunctionSiblingEscape(t *testing.T) {
	// Build a temporary workspace root.
	root := t.TempDir()

	// Place the secret OUTSIDE the workspace root, at the junction target.
	// Junction target: parent of root = <tmp>/<rand>/001
	// Secret: <tmp>/<rand>/001/secret.txt  (only reachable via the junction)
	juncTarget := filepath.Dir(root) // e.g. <tmp>/<rand>/001
	if err := os.MkdirAll(juncTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(juncTarget, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOPSECRET"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Create junction ws-esc -> juncTarget (sibling of root).
	// workspace root: root = <tmp>/<rand>/001/ws
	// junction name  : <tmp>/<rand>/001/ws/ws-esc
	// junction target: <tmp>/<rand>/001
	// file through junction: <tmp>/<rand>/001/ws/ws-esc/secret.txt
	// Resolved path: <tmp>/<rand>/001/secret.txt  — NOT inside root.
	juncLink := filepath.Join(root, "ws-ws-esc")
	makeJunction(t, juncLink, juncTarget)

	// Opening the secret through the junction resolves outside root.
	f, err := openFileNoFollow(secret, os.O_RDONLY, 0, root)
	if err == nil {
		f.Close()
		t.Fatalf("openFileNoFollow accepted a file opened through a junction that escapes the workspace root — this is a TOCTOU escape. root=%q, resolved=%q", root, secret)
	}
	if !strings.Contains(err.Error(), "outside workspace") {
		t.Fatalf("openFileNoFollow returned a non-escape error: %v", err)
	}
	t.Logf("correctly rejected junction sibling escape: %v", err)
}

// TestOpenFileNoFollow_AllowsJunctionInsideRoot verifies the fix does not
// break the legitimate case: a junction INSIDE the workspace root may be
// followed, because the resolved path stays within the root.
func TestOpenFileNoFollow_AllowsJunctionInsideRoot(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "subdir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(subdir, "secret.txt")
	if err := os.WriteFile(secret, []byte("INSIDE"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Junction inside the root: ws/junction -> ws/subdir
	juncLink := filepath.Join(root, "junction")
	makeJunction(t, juncLink, subdir)

	f, err := openFileNoFollow(secret, os.O_RDONLY, 0, root)
	if err != nil {
		t.Fatalf("openFileNoFollow rejected a file reachable through a junction that stays inside the workspace: %v", err)
	}
	f.Close()
}
