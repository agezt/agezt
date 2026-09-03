// SPDX-License-Identifier: MIT

//go:build !windows

package file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestReadSymlinkTOCTOU_Rejected proves that a file.Tool created with a
// workspace root cannot be tricked into reading a file outside that root by
// planting a symlink after tool construction.
//
// Vulnerability: doRead (and doSearch, doReplace, doReadRange) previously
// used os.ReadFile/os.Open directly, bypassing the openFileNoFollow
// backstop.  A symlink/junction planted inside the workspace after
// tool construction could be followed on the next read, exposing files
// outside the workspace root — a TOCTOU escape of the EvalSymlinks
// containment check performed in New.
//
// Fix: all read-path opens in file.go now route through openFileNoFollow,
// which uses O_NOFOLLOW (unix) / GetFinalPathNameByHandle with Rel-check
// (windows) to detect and reject resolved paths outside the workspace root.
// Before the fix this test FAILS (reads the outside file); after the fix
// it PASSES (error returned).
func TestReadSymlinkTOCTOU_Rejected(t *testing.T) {
	// 1. Create the workspace root.
	wsRoot := t.TempDir()
	tool, err := New(wsRoot)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Create a subdirectory and a secret file OUTSIDE the workspace.
	outsideDir := t.TempDir()
	secretFile := filepath.Join(outsideDir, "secret.txt")
	const secret = "TOPSECRET-FILE-CONTENTS-READ-IF-VULNERABLE"
	if err := os.WriteFile(secretFile, []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}

	// 3. Plant a symlink INSIDE the workspace pointing to the outside dir.
	// On Linux  : symlink
	// On Windows: junction (requires admin/DevMode; skip on failure — the
	//             unit test in nofollow_windows_test.go covers junctions).
	symlinkPath := filepath.Join(wsRoot, "escape-hatch")
	linkErr := os.Symlink(outsideDir, symlinkPath)
	if linkErr != nil {
		t.Skipf("cannot create symlink (may need CAP_SYS_ADMIN or Dev Mode): %v", linkErr)
	}

	// 4. Call the read op for the file as reached through the symlink.
	//    Path is relative to the workspace root.
	relativePath := filepath.ToSlash(filepath.Join("escape-hatch", "secret.txt"))
	input := fileInput{Op: "read", Path: relativePath}
	rawIn, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Invoke(context.Background(), rawIn)
	if err == nil && result.Output == "" {
		// No error AND no output — the read silently succeeded (the vulnerable
		// os.ReadFile path returned the outside file).  If the fix is applied
		// (openFileNoFollow with O_NOFOLLOW), the open fails with an
		// "outside workspace" error and we reach the t.Fatalf below.
		t.Fatalf("BUG-DEMONSTRATED: read of %q (through symlink to %q) succeeded — file contents exposed outside workspace root %q. Expected an error because openFileNoFollow must reject resolved paths outside the root.", relativePath, secretFile, wsRoot)
	}

	// Fix is present: the open was rejected.
	if err != nil {
		t.Logf("FIXED: openFileNoFollow rejected the through-symlink read (as expected): %v", err)
	} else if result.Output == "" {
		t.Fatalf("BUG: read returned empty output but no error — check the read-path open logic")
	} else {
		t.Fatalf("BUG: read returned unexpected output: %q", result.Output)
	}
}

// TestSearchSymlinkTOCTOU_Rejected proves that doSearch also routes through
// openFileNoFollow and rejects resolved paths outside the workspace root.
func TestSearchSymlinkTOCTOU_Rejected(t *testing.T) {
	wsRoot := t.TempDir()
	tool, err := New(wsRoot)
	if err != nil {
		t.Fatal(err)
	}

	outsideDir := t.TempDir()
	secretFile := filepath.Join(outsideDir, "target.txt")
	const secret = "SEARCHABLE-TOPSECRET"
	if err := os.WriteFile(secretFile, []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(wsRoot, "escape")
	if err := os.Symlink(outsideDir, symlinkPath); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	// Search for the secret string — the vulnerable code would scan the
	// outside file and return a hit.
	input := fileInput{Op: "search", Path: "", Pattern: secret}
	rawIn, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Invoke(context.Background(), rawIn)
	if err == nil && result.Output != "" && len(result.Output) > len("no matches") {
		t.Fatalf("BUG-DEMONSTRATED: search through symlink found secret %q in a file outside workspace root %q. Expected openFileNoFollow to reject the read.", secret, wsRoot)
	}
	t.Logf("FIXED: search correctly rejected or found no matches (output=%q, err=%v)", result.Output, err)
}

// TestReplaceSymlinkTOCTOU_Rejected proves that doReplace also routes through
// openFileNoFollow and cannot be used to read/rewrite a file outside the
// workspace via a planted symlink.
func TestReplaceSymlinkTOCTOU_Rejected(t *testing.T) {
	wsRoot := t.TempDir()
	tool, err := New(wsRoot)
	if err != nil {
		t.Fatal(err)
	}

	outsideDir := t.TempDir()
	targetFile := filepath.Join(outsideDir, "replace-target.txt")
	if err := os.WriteFile(targetFile, []byte("ORIGINAL-CONTENT"), 0o600); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(wsRoot, "replace-escape")
	if err := os.Symlink(outsideDir, symlinkPath); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	relativePath := filepath.ToSlash(filepath.Join("replace-escape", "replace-target.txt"))
	input := fileInput{Op: "replace", Path: relativePath, Find: "ORIGINAL", Replacement: "REPLACED"}
	rawIn, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Invoke(context.Background(), rawIn)
	if err == nil {
		// Read the actual outside file to see if it was modified.
		got, _ := os.ReadFile(targetFile)
		if string(got) != "ORIGINAL-CONTENT" {
			t.Fatalf("BUG-DEMONSTRATED: doReplace modified file outside workspace root. Expected openFileNoFollow to reject the read-open. File content: %q", string(got))
		}
	}
	t.Logf("FIXED: replace correctly rejected the through-symlink open (result=%q, err=%v)", result.Output, err)
}
