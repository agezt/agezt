// SPDX-License-Identifier: MIT

//go:build unix

package file

import (
	"os"
	"path/filepath"
	"testing"
)

// TestONoFollow_RefusesSymlink proves the constant the write paths use is the
// real O_NOFOLLOW flag: opening a symlink with it must fail (ELOOP), which is
// what closes the resolve()→open TOCTOU in doWrite/doReplace. This is the
// security guarantee those paths rely on; it runs on Unix only (O_NOFOLLOW is a
// POSIX flag — the non-Unix build uses a 0 no-op, see nofollow_other.go).
//
// Negative control: if oNoFollow were 0 (the no-op fallback), the open below
// would SUCCEED and follow the link, and this test would fail — so the test
// directly exercises that the wired constant has teeth.
func TestONoFollow_RefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported here: %v", err)
	}

	// The exact flag combination doWrite uses for a fresh write.
	f, err := os.OpenFile(link, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|oNoFollow, 0o644)
	if err == nil {
		f.Close()
		t.Fatal("O_NOFOLLOW open of a symlink succeeded — the TOCTOU guard is inert")
	}

	// Sanity: the same open WITHOUT O_NOFOLLOW would have followed the link (this
	// is the behavior the guard prevents). Confirms the symlink itself is openable.
	f2, err := os.OpenFile(link, os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("control open (no O_NOFOLLOW) should follow the link: %v", err)
	}
	f2.Close()
}
