// SPDX-License-Identifier: MIT

//go:build !windows

package file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	switch {
	case err != nil:
		// Refused via the Go error channel — openFileNoFollow/resolve
		// rejected the resolved-outside path.
		t.Logf("FIXED: through-symlink read refused via error (as expected): %v", err)
	case result.IsError:
		// Refused via the result channel — the tool reports refusals as
		// agent.Result{IsError: true} (e.g. resolve()'s containment check),
		// not as a Go error. The message names the resolved path; the FILE
		// CONTENTS must still not appear.
		if strings.Contains(result.Output, secret) {
			t.Fatalf("BUG-DEMONSTRATED: the refusal message leaked the outside file contents: %q", result.Output)
		}
		t.Logf("FIXED: through-symlink read refused via the result channel (as expected): %s", result.Output)
	default:
		t.Fatalf("BUG-DEMONSTRATED: read of %q (through symlink to %q) was not refused (output=%q) — contents outside workspace root %q must not be readable", relativePath, secretFile, result.Output, wsRoot)
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
	switch {
	case err != nil:
		t.Logf("FIXED: search refused via the error channel (as expected): %v", err)
		return
	case result.IsError:
		t.Logf("FIXED: search refused via the result channel (as expected): %s", result.Output)
		return
	}
	// The search ran. The hits envelope embeds the PATTERN — which IS the
	// secret string — so a substring test would self-trip; assert on the
	// hit count instead.
	var body struct {
		Count int `json:"count"`
	}
	if jerr := json.Unmarshal([]byte(result.Output), &body); jerr != nil {
		t.Fatalf("BUG: unexpected search output (not a hits envelope): %q", result.Output)
	}
	if body.Count > 0 {
		t.Fatalf("BUG-DEMONSTRATED: search returned %d hit(s) for %q through the escape symlink — contents outside workspace root %q leaked", body.Count, secret, wsRoot)
	}
	t.Logf("FIXED: search found no hits through the symlink (count=0)")
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
