// SPDX-License-Identifier: MIT
//
// Round-6 elite-bug-hunter reproduction: kernel/skill/bundle.go
// in-bundle symlink escape during Read.
//
// BUG: kernel/skill/bundle.go, Read(name, rel)
//
//   func (b *BundleStore) Read(name, rel string) ([]byte, error) {
//       cleaned, err := cleanRel(rel)   // validates the REQUESTED path
//       full := filepath.Join(b.root, slug, cleaned)
//       data, err := os.ReadFile(full)  // FOLLOWS symlinks — bypasses cleanRel!
//       return data, nil
//   }
//
// cleanRel validates the caller's requested path (no .., no absolute), but
// os.ReadFile follows symlinks at the OS level. A symlink planted inside the
// bundle directory (e.g. bundle/scripts/leak → /etc/passwd) is invisible to
// cleanRel and will be resolved by the kernel before the read happens.
//
// PLANT VECTOR: OpenBundles/Write create directories with 0755 (world-readable).
// Any local user can os.Symlink into a bundle's directory.
//
// ATTACK PATH (agent-operated via op=read):
//   1. op=files → agent sees scripts/leak listed (WalkDir follows symlinks)
//   2. op=read → agent requests scripts/leak
//   3. bundles.Read() → os.ReadFile resolves the link → arbitrary file content
//      returned and disclosed in the JSON response.
package skill

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBundleRead_FollowsSymlinkInsideBundle proves that a symlink planted
// inside a bundle directory is followed by Read, bypassing cleanRel.
//
// VULNERABILITY: cleanRel blocks ../etc/passwd as a REQUESTED path, but
// bundle/scripts/leak → /etc/passwd bypasses it silently. Read returns
// the target file's content. On a shared-host daemon, any local user can
// plant the symlink because OpenBundles creates <root>/bundles/<slug>/ at 0755.
func TestBundleRead_FollowsSymlinkInsideBundle(t *testing.T) {
	// Set up the bundle store and seed a bundle with one legitimate file.
	bs, err := OpenBundles(t.TempDir())
	if err != nil {
		t.Fatalf("OpenBundles: %v", err)
	}
	files := map[string][]byte{
		"scripts/setup.sh": []byte("#!/bin/sh\necho hi"),
	}
	_, err = bs.Write("my-skill", files)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Plant a symlink inside the bundle directory, pointing to an outside file.
	// This simulates a local untrusted user or compromised process writing to
	// the world-readable bundle directory.
	bundleDir := bs.Dir("my-skill")
	leakPath := filepath.Join(bundleDir, "scripts", "leak")
	outside := filepath.Join(t.TempDir(), "outside_target.txt")
	if err := os.WriteFile(outside, []byte("SENSITIVE CONTENT"), 0o644); err != nil {
		t.Fatalf("write outside target: %v", err)
	}
	if err := os.Symlink(outside, leakPath); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// Confirm the symlink is NOT blocked by cleanRel as a requested path.
	// (The request path "scripts/leak" is a valid, non-escaping relative path.)
	cleaned, err := cleanRel("scripts/leak")
	if err != nil {
		t.Fatalf("cleanRel should accept scripts/leak: %v", err)
	}
	t.Logf("cleanRel accepts the path: %q", cleaned)

	// Confirm the symlink shows up in List() (WalkDir follows symlinks).
	rels, err := bs.List("my-skill")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var hasLink bool
	for _, r := range rels {
		if r == "scripts/leak" {
			hasLink = true
			break
		}
	}
	if !hasLink {
		t.Fatalf("WalkDir should have surfaced scripts/leak via the symlink (got %v)", rels)
	}

	// The vulnerability: Read follows the planted symlink and returns the
	// outside target's content instead of an error.
	data, err := bs.Read("my-skill", "scripts/leak")

	// EXPECTED (correct) behavior: Read should return an error because the
	// resolved path escapes the bundle root. Since /etc/passwd is outside
	// <root>/bundles/my-skill/, Read must refuse to follow the link.
	// ACTUAL (buggy) behavior: Read returns nil error and the outside file's
	// content, silently bypassing the cleanRel guard.
	if err == nil {
		t.Errorf("Read(scripts/leak) returned nil error (followed the planted symlink)")
		t.Errorf("  Root cause: bundle.go Read calls os.ReadFile(full) without")
		t.Errorf("  verifying the resolved path stays inside b.root.")
		t.Errorf("  os.ReadFile resolves symlinks at the OS level, bypassing cleanRel.")
		t.Errorf("  cleanRel only validated the REQUESTED path, not the resolved path.")
		t.Errorf("  Expected: a non-nil error (symlink target outside bundle root)")
		t.Errorf("  Actual: data=%q (the outside file's content!)", string(data))
		if string(data) == "SENSITIVE CONTENT" {
			t.Errorf("  CONFIRMED: bundle Read leaked the outside file content: %q", string(data))
		}
	} else {
		// Fix confirmed: Read rejected the symlink escape.
		t.Logf("Read correctly rejected the symlink: %v", err)
	}
}
