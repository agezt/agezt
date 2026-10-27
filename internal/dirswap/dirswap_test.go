// SPDX-License-Identifier: MIT

package dirswap

// Provenance: the guarantees of the shared directory-swap policy (W5.4) —
//             the one place a tree is published, so the guarantee lives here
//             and is asserted here rather than in each caller's test.

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeTree writes a small tree under root.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}

// readFile reads a file and fails the test when it cannot be read.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// TestTheSwapPublishesTheStagedTree: the staged tree becomes the destination,
// and the tree it replaced is gone.
func TestTheSwapPublishesTheStagedTree(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "tree")
	writeTree(t, dst, map[string]string{"index.json": "old"})

	staging, err := Stage(dst)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	writeTree(t, staging, map[string]string{"index.json": "new"})

	if err := Swap(staging, dst); err != nil {
		t.Fatalf("Swap: %v", err)
	}
	if got := readFile(t, filepath.Join(dst, "index.json")); got != "new" {
		t.Fatalf("the published tree holds %q, want %q", got, "new")
	}
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Fatalf("the staging tree still exists after the swap (stat err = %v)", err)
	}
}

// TestTheSwapKeepsTheOldTreeWhenThePublishFails is THE guarantee: a publish
// that fails must leave the destination exactly as it was.
//
// The old policy deleted the destination FIRST and renamed the new tree over
// it, so a rename that failed left the caller with NOTHING — the content was
// deleted, not moved, and could not be recovered.
func TestTheSwapKeepsTheOldTreeWhenThePublishFails(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "tree")
	writeTree(t, dst, map[string]string{"index.json": "old"})

	// A staged tree that does not exist: the publish cannot succeed.
	err := Swap(filepath.Join(root, "missing"), dst)
	if err == nil {
		t.Fatal("the swap of a missing tree must fail")
	}
	if got := readFile(t, filepath.Join(dst, "index.json")); got != "old" {
		t.Fatalf("the destination holds %q after a failed swap, want the previous %q", got, "old")
	}
	if _, err := os.Stat(AsidePath(dst)); !os.IsNotExist(err) {
		t.Fatalf("the rollback left the previous tree at %s (stat err = %v)", AsidePath(dst), err)
	}
}

// TestTheSwapPublishesATreeThatDidNotExistBefore: the first publish creates
// the destination.
func TestTheSwapPublishesATreeThatDidNotExistBefore(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "tree")
	staging, err := Stage(dst)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	writeTree(t, staging, map[string]string{"index.json": "first"})

	if err := Swap(staging, dst); err != nil {
		t.Fatalf("Swap: %v", err)
	}
	if got := readFile(t, filepath.Join(dst, "index.json")); got != "first" {
		t.Fatalf("the published tree holds %q, want %q", got, "first")
	}
}

// TestTheSwapLeavesNoLeftoverBehind: the tree the swap replaced is removed,
// and it does not linger next to the destination.
func TestTheSwapLeavesNoLeftoverBehind(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "tree")
	writeTree(t, dst, map[string]string{"index.json": "old"})
	staging, err := Stage(dst)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	writeTree(t, staging, map[string]string{"index.json": "new"})

	if err := Swap(staging, dst); err != nil {
		t.Fatalf("Swap: %v", err)
	}
	if _, err := os.Stat(AsidePath(dst)); !os.IsNotExist(err) {
		t.Fatalf("the replaced tree still lingers at %s (stat err = %v)", AsidePath(dst), err)
	}
}

// TestTheSwapClearsTheLeftoverOfAnEarlierAttempt: the previous tree always
// sits at ONE path next to the destination, so a destination can never
// accumulate more than one leftover — and a reader that lists the directory
// never sees it as a published tree.
func TestTheSwapClearsTheLeftoverOfAnEarlierAttempt(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "tree")
	writeTree(t, dst, map[string]string{"index.json": "old"})
	writeTree(t, AsidePath(dst), map[string]string{"index.json": "stale"})

	staging, err := Stage(dst)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	writeTree(t, staging, map[string]string{"index.json": "new"})

	if err := Swap(staging, dst); err != nil {
		t.Fatalf("Swap: %v", err)
	}
	if _, err := os.Stat(AsidePath(dst)); !os.IsNotExist(err) {
		t.Fatalf("the leftover of the earlier attempt is still there (stat err = %v)", err)
	}
	if got := readFile(t, filepath.Join(dst, "index.json")); got != "new" {
		t.Fatalf("the published tree holds %q, want %q", got, "new")
	}
}

// TestTheSwapRefusesToPublishAPathOntoItself: the guard that stops a
// caller from handing the policy the same path twice — which would move the
// tree onto itself and destroy it.
func TestTheSwapRefusesToPublishAPathOntoItself(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "tree")
	writeTree(t, dst, map[string]string{"index.json": "old"})

	if err := Swap(dst, dst); err == nil {
		t.Fatal("the swap of a path onto itself must fail")
	}
	if got := readFile(t, filepath.Join(dst, "index.json")); got != "old" {
		t.Fatalf("the destination holds %q after a refused swap, want %q", got, "old")
	}
}

// TestTheStageClearsTheLeftoverOfAnEarlierAttempt: a staging tree from an
// interrupted attempt is removed, so a new attempt never publishes half of
// the old one.
func TestTheStageClearsTheLeftoverOfAnEarlierAttempt(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "tree")
	stale := StagingPath(dst)
	writeTree(t, stale, map[string]string{"index.json": "stale"})

	staging, err := Stage(dst)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if staging != stale {
		t.Fatalf("the staging path = %q, want %q", staging, stale)
	}
	if _, err := os.Stat(filepath.Join(staging, "index.json")); !os.IsNotExist(err) {
		t.Fatalf("the staging tree still holds the previous attempt's file (stat err = %v)", err)
	}
}

// TestTheWritePublishesTheWholeFile: the file that lands is the file that
// was written — the writer goes through the platform's atomic writer, so a
// reader never sees a half-written file.
func TestTheWritePublishesTheWholeFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "index.json")
	want := `{"name":"marketplace","packs":["a","b"]}`

	if err := WriteFile(path, []byte(want), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := readFile(t, path); got != want {
		t.Fatalf("the file holds %q, want %q", got, want)
	}
}

// TestTheWriteReassertsTheMode: the mode is the FILE's, not the property of
// whoever wrote last.
//
// os.WriteFile applies the mode only on CREATE, so a file left behind by an
// earlier run kept its old mode — and a leftover read-only file made the
// write fail outright ("Access is denied" on Windows).
func TestTheWriteReassertsTheMode(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "index.json")

	if err := WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatalf("the first write: %v", err)
	}
	// Leave a read-only file behind, the way an earlier run would.
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := WriteFile(path, []byte("second"), 0o600); err != nil {
		t.Fatalf("the second write must succeed over a read-only file: %v", err)
	}
	if got := readFile(t, path); got != "second" {
		t.Fatalf("the file holds %q, want %q", got, "second")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	mode := info.Mode().Perm()
	// The guarantee is portable: the file is WRITABLE again. Windows has no
	// POSIX permission bits — a writable file reports 0666 and a read-only
	// one 0444 — so the exact mode is only asserted where it exists.
	if mode&0o200 == 0 {
		t.Fatalf("the file's mode = %o — it is not writable, so the next write would fail outright", mode)
	}
	if runtime.GOOS != "windows" && mode != 0o600 {
		t.Fatalf("the file's mode = %o, want 600 — the mode must be re-asserted on every write", mode)
	}
}

// TestTheWriteLeavesNoTempFileBehind: the writer's temp file does not stay
// in the published tree.
func TestTheWriteLeavesNoTempFileBehind(t *testing.T) {
	root := t.TempDir()
	if err := WriteFile(filepath.Join(root, "index.json"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read the directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "index.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("the directory holds %v, want only index.json", names)
	}
}

// TestATransientNameIsNotAPublishedTree: a reader that lists a directory has
// to skip the working directories, or it offers the reader a stale copy of
// something that was replaced.
func TestATransientNameIsNotAPublishedTree(t *testing.T) {
	transient := []string{"acme.staging", "acme.old"}
	published := []string{"acme", "acme2", "staging", "older"}
	for _, name := range transient {
		if !IsTransient(name) {
			t.Errorf("%q is a working directory, not a published one", name)
		}
	}
	for _, name := range published {
		if IsTransient(name) {
			t.Errorf("%q is a published tree, not a working directory", name)
		}
	}
}
