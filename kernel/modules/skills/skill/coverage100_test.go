// SPDX-License-Identifier: MIT

package skill

import (
	"strings"
	"testing"
)

// TestBundleStore_SettersGettersAndRemove drives the previously-uncovered
// BundleStore.Remove / Dir paths and the Forge.SetBundles/Bundles accessors.
func TestForgeBundleAccessors(t *testing.T) {
	f, _ := newTestForge(t)

	b, err := OpenBundles(t.TempDir())
	if err != nil {
		t.Fatalf("OpenBundles: %v", err)
	}
	// SetBundles then Bundles() should round-trip.
	f.SetBundles(b)
	if f.Bundles() != b {
		t.Fatalf("Bundles() should return the wired store")
	}
	f.SetBundles(nil)
	if f.Bundles() != nil {
		t.Fatalf("Bundles() should be nil after SetBundles(nil)")
	}
}

func TestBundleStoreDirWriteReadListRemove(t *testing.T) {
	b, err := OpenBundles(t.TempDir())
	if err != nil {
		t.Fatalf("OpenBundles: %v", err)
	}
	const name = "PDF Fill"

	// Dir must be deterministic for a name.
	dir := b.Dir(name)
	if dir == "" {
		t.Fatalf("Dir returned empty path")
	}

	// Write a couple of resources.
	rels, err := b.Write(name, map[string][]byte{
		"scripts/setup.sh": []byte("#!/bin/sh\necho hi\n"),
		"reference.md":     []byte("# ref\n"),
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if len(rels) != 2 {
		t.Fatalf("expected 2 rels, got %v", rels)
	}

	// List should return the same (sorted) set.
	got, err := b.List(name)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 listed rels, got %v", got)
	}

	// Read one back.
	data, err := b.Read(name, "reference.md")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(string(data), "ref") {
		t.Fatalf("unexpected read content: %q", data)
	}

	// Remove clears the bundle; a second Remove on an absent bundle is a no-op.
	if err := b.Remove(name); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := b.Remove(name); err != nil {
		t.Fatalf("Remove (absent) should be nil, got %v", err)
	}
	// After removal, List returns nothing.
	got, err = b.List(name)
	if err != nil {
		t.Fatalf("List after remove: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty list after remove, got %v", got)
	}

	// Remove with an unslugifiable name is a no-op (empty slug path).
	if err := b.Remove("   "); err != nil {
		t.Fatalf("Remove empty-slug should be nil, got %v", err)
	}
}

func TestBundleStoreListEmptyName(t *testing.T) {
	b, err := OpenBundles(t.TempDir())
	if err != nil {
		t.Fatalf("OpenBundles: %v", err)
	}
	if got, err := b.List("   "); err != nil || got != nil {
		t.Fatalf("List(empty slug) = %v, %v; want nil, nil", got, err)
	}
}

func TestForgeArchiveAndRestoreStatus(t *testing.T) {
	f, _ := newTestForge(t)

	sk, _, err := f.Create("corr", CreateSpec{Name: "arch-me", Body: "body"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Archive a freshly-created (shadow) skill.
	if err := f.Archive("corr", sk.ID, "no longer needed"); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	// Archiving again is idempotent (already archived -> nil).
	if err := f.Archive("corr", sk.ID, "again"); err != nil {
		t.Fatalf("Archive idempotent: %v", err)
	}
	// Archive of an unknown id errors.
	if err := f.Archive("corr", "missing", "x"); err == nil {
		t.Fatalf("Archive(missing) should error")
	}

	// RestoreStatus with an invalid status errors.
	if _, _, err := f.RestoreStatus("corr", sk.ID, Status("bogus"), ""); err == nil {
		t.Fatalf("RestoreStatus(invalid) should error")
	}
	// RestoreStatus with a valid status bypasses the forward-only matrix.
	from, to, err := f.RestoreStatus("corr", sk.ID, StatusShadow, "rollback")
	if err != nil {
		t.Fatalf("RestoreStatus: %v", err)
	}
	if from != StatusArchived || to != StatusShadow {
		t.Fatalf("RestoreStatus from/to = %q/%q; want archived/shadow", from, to)
	}
	// RestoreStatus with empty reason (skips the reason payload branch).
	if _, _, err := f.RestoreStatus("corr", sk.ID, StatusActive, ""); err != nil {
		t.Fatalf("RestoreStatus(empty reason): %v", err)
	}
	// RestoreStatus of an unknown id errors.
	if _, _, err := f.RestoreStatus("corr", "missing", StatusShadow, ""); err == nil {
		t.Fatalf("RestoreStatus(missing) should error")
	}
}
