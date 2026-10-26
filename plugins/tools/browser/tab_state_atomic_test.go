// SPDX-License-Identifier: MIT

package browser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The tab state is the browser session's memory: it holds the final URL of a
// tab and the snapshot refs a later call can reuse. It is READ BACK by later
// `browser.action` calls (to resolve a tab_id's URL and its refs) and it
// survives daemon restarts — it lives under the session root, not in memory.
//
// There is no mutex around the save and the read, so two concurrent
// `browser.action` invocations can interleave. A write that truncates in
// place leaves a window in which the state file is short: a state that does
// not parse fails the next call loudly, but a state that DOES parse can
// carry a truncated URL — and the driver would then navigate to it.
//
// The platform's atomic writer (internal/atomicfile) replaces the state file
// instead, so a reader always sees a whole state — the old one or the new
// one — and the mode is re-asserted on every write.

// fileIdentity opens path and returns the identity of the OPEN file.
//
// The identity has to come from an open handle: os.Stat on Windows returns the
// same identity for a replaced path (the path stat does not carry the file
// index), while the handle's own stat does.
func fileIdentity(t *testing.T, path string) os.FileInfo {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info
}

// ownerWritable reports whether the file at path may be written by its owner.
//
// It is the portable form of "the mode was re-asserted": on POSIX the owner
// write bit, and on Windows the read-only attribute (Go maps a mode without
// the owner write bit onto the read-only attribute).
func ownerWritable(t *testing.T, path string) bool {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()&0o200 != 0
}

// tabStateTool returns an action tool rooted at a fresh session root.
func tabStateTool(t *testing.T) *ActionTool {
	t.Helper()
	return &ActionTool{SessionRoot: t.TempDir()}
}

// TestTheTabStateIsReplacedNotRewritten: the state file is REPLACED by a
// rename, not rewritten in place. A reader that has the file open keeps
// reading the file it opened — it never sees a torn state.
func TestTheTabStateIsReplacedNotRewritten(t *testing.T) {
	tool := tabStateTool(t)
	if err := tool.saveTabState("s1", "tab1", "https://example.com/first", nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	path, err := tool.tabStatePath("s1", "tab1")
	if err != nil {
		t.Fatal(err)
	}
	before := fileIdentity(t, path)

	if err := tool.saveTabState("s1", "tab1", "https://example.com/second", []actionSnapshotRef{{Ref: "r1", Selector: "css=a"}}); err != nil {
		t.Fatalf("second save: %v", err)
	}
	after := fileIdentity(t, path)

	if os.SameFile(before, after) {
		t.Fatal("the tab state was rewritten in place; a reader can see a torn state")
	}
	st, err := tool.readTabState("s1", "tab1")
	if err != nil {
		t.Fatal(err)
	}
	if st.URL != "https://example.com/second" {
		t.Fatalf("saved URL = %q, want %q", st.URL, "https://example.com/second")
	}
	if len(st.Snapshot) != 1 || st.Snapshot[0].Ref != "r1" {
		t.Fatalf("saved refs = %+v", st.Snapshot)
	}
}

// TestTheTabStateIsWholeWhileAReaderHoldsItOpen: the guarantee is not only
// about a reader that arrives after the write. A reader that ALREADY holds
// the file open keeps a consistent view: it never reads a truncated state.
//
// The OUTCOME of the rewrite differs by platform, and both outcomes are
// correct:
//
//   - POSIX: the rename succeeds over the open file, the path carries the new
//     content and the held handle keeps the old one.
//   - Windows: the platform's atomic writer REFUSES to replace a file another
//     handle holds open (rename fails with "Access is denied", and its
//     remove-then-rename fallback cannot remove a file that is open either),
//     so the write is refused and the old file stays whole.
//
// The invariant both share is the one a reader depends on: the held handle
// reads a COMPLETE file, and the path holds a COMPLETE file.
func TestTheTabStateIsWholeWhileAReaderHoldsItOpen(t *testing.T) {
	tool := tabStateTool(t)
	if err := tool.saveTabState("s1", "tab1", "https://example.com/first", nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	path, err := tool.tabStatePath("s1", "tab1")
	if err != nil {
		t.Fatal(err)
	}
	held, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	if err := tool.saveTabState("s1", "tab1", "https://example.com/second", nil); err != nil {
		// Windows refuses the replacement while a reader holds the file;
		// the old file stays whole, which is the invariant.
		t.Log("the rewrite was refused while a reader held the file open")
	}

	buf := make([]byte, 512)
	n, err := held.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	heldState := string(buf[:n])
	if !strings.Contains(heldState, "https://example.com/first") {
		t.Fatalf("the held handle reads %q; a reader can see a torn state", heldState)
	}
	// The path holds a whole state too: the new one where the rename went
	// through, the old one where the write was refused.
	onPath, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(onPath), "https://example.com/") {
		t.Fatalf("the tab state is torn: %q", string(onPath))
	}
	st, err := tool.readTabState("s1", "tab1")
	if err != nil {
		t.Fatalf("the state on disk does not parse: %v", err)
	}
	if st.URL != "https://example.com/first" && st.URL != "https://example.com/second" {
		t.Fatalf("saved URL = %q", st.URL)
	}
}

// TestTheTabStateIsWritableByTheirOwner: the state file survives across calls
// and across restarts, so a leftover file from an earlier run must not be
// able to stop the next save. os.WriteFile applies its mode only when it
// CREATES the file, and opening a read-only file for writing FAILS — on
// POSIX with EACCES, on Windows with "Access is denied" — so a leftover
// read-only state would stop the browser from recording the tab at all.
//
// The atomic writer writes a fresh temp file, chmods it to the requested mode
// and renames it over the target, so the result is always writable by its
// owner — and the write never depends on the old file's mode.
func TestTheTabStateIsWritableByTheirOwner(t *testing.T) {
	tool := tabStateTool(t)
	if err := tool.saveTabState("s1", "tab1", "https://example.com/first", nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	path, err := tool.tabStatePath("s1", "tab1")
	if err != nil {
		t.Fatal(err)
	}
	// A leftover state from an earlier run that nobody may write.
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}

	if err := tool.saveTabState("s1", "tab1", "https://example.com/second", nil); err != nil {
		t.Fatalf("a leftover read-only state stopped the save: %v", err)
	}
	if !ownerWritable(t, path) {
		t.Fatal("the tab state is not writable by its owner")
	}
	st, err := tool.readTabState("s1", "tab1")
	if err != nil {
		t.Fatal(err)
	}
	if st.URL != "https://example.com/second" {
		t.Fatalf("saved URL = %q", st.URL)
	}
}

// TestTheTabStateIsPrivateToTheirOwner: the state holds the URLs the session
// visited and the refs it captured, and it is written 0600.
//
// The permission BITS are a POSIX property; Windows has no group/other
// distinction, so this check runs where the bits exist.
func TestTheTabStateIsPrivateToTheirOwner(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("POSIX permission bits")
	}
	tool := tabStateTool(t)
	if err := tool.saveTabState("s1", "tab1", "https://example.com/first", nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	path, err := tool.tabStatePath("s1", "tab1")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Fatalf("the tab state is readable by others: %v", mode)
	}
}

// TestTheTabStatePathStaysUnderTheSessionRoot: the state file is the
// session's private file; it must not escape the session root.
func TestTheTabStatePathStaysUnderTheSessionRoot(t *testing.T) {
	tool := tabStateTool(t)
	if err := tool.saveTabState("s1", "tab1", "https://example.com/first", nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	path, err := tool.tabStatePath("s1", "tab1")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(tool.SessionRoot)
	if err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(abs, root) {
		t.Fatalf("the tab state escaped the session root: %s", abs)
	}
}
