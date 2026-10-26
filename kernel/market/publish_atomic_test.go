// SPDX-License-Identifier: MIT

package market

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The published marketplace is a PUBLIC artifact: the directory the operator
// publishes into is meant to be served statically, and remote clients fetch
// `marketplace.json` and the pack bodies from it. The writer also reads the
// index back on the next publish, so the index is written AND read by two
// different parties.
//
// A write that truncates in place leaves a window in which the file is
// short. A client that fetches a half-written index gets invalid JSON and its
// sync fails; a client that fetches a half-written pack cannot verify it. And
// the writer's own read-back would see a half-written index — which the old
// code treated as "no catalogue at all", silently dropping every pack
// published before it.
//
// The platform's atomic writer (internal/atomicfile, through this package's
// atomicWrite) replaces the file instead, so a client never fetches a
// half-written file, the next publish never reads one, and the mode is
// re-asserted on every write.

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

// publishedPack builds a pack for the publish path.
func publishedPack(t *testing.T, name, version string) Pack {
	t.Helper()
	p, err := BuildPackFromDir(writePackDir(t))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	p.Name = name
	p.Version = version
	return p
}

// TestThePublishedFilesAreReplacedNotRewritten: the published pack and the
// index are REPLACED by a rename, not rewritten in place. A client that
// fetches the file sees one of two whole files — never a half-written one.
func TestThePublishedFilesAreReplacedNotRewritten(t *testing.T) {
	out := t.TempDir()
	if err := Publish(publishedPack(t, "demo-pack", "2.1.0"), out, "acme", nil, 1); err != nil {
		t.Fatalf("publish: %v", err)
	}
	packPath := filepath.Join(out, "packs", "demo-pack.json")
	idxPath := filepath.Join(out, "marketplace.json")
	packBefore := fileIdentity(t, packPath)
	idxBefore := fileIdentity(t, idxPath)

	if err := Publish(publishedPack(t, "demo-pack", "2.2.0"), out, "acme", nil, 2); err != nil {
		t.Fatalf("re-publish: %v", err)
	}
	if os.SameFile(packBefore, fileIdentity(t, packPath)) {
		t.Fatal("the published pack was rewritten in place; a client can fetch a half-written pack")
	}
	if os.SameFile(idxBefore, fileIdentity(t, idxPath)) {
		t.Fatal("the published index was rewritten in place; a client can fetch a half-written index")
	}

	// The content is the NEW one.
	body, err := os.ReadFile(packPath)
	if err != nil {
		t.Fatal(err)
	}
	var published Pack
	if err := json.Unmarshal(body, &published); err != nil {
		t.Fatalf("the published pack is not parseable: %v", err)
	}
	if published.Version != "2.2.0" {
		t.Fatalf("published version = %q, want %q", published.Version, "2.2.0")
	}
	idx, err := os.ReadFile(idxPath)
	if err != nil {
		t.Fatal(err)
	}
	var mp Marketplace
	if err := json.Unmarshal(idx, &mp); err != nil {
		t.Fatalf("the published index is not parseable: %v", err)
	}
	if len(mp.Packs) != 1 || mp.Packs[0].Name != "demo-pack" {
		t.Fatalf("index = %+v", mp.Packs)
	}
}

// TestThePublishedIndexKeepsEveryPublishedPack: the index is the CATALOGUE.
// Publishing a second pack must not drop the first one.
func TestThePublishedIndexKeepsEveryPublishedPack(t *testing.T) {
	out := t.TempDir()
	if err := Publish(publishedPack(t, "demo-pack", "2.1.0"), out, "acme", nil, 1); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := Publish(publishedPack(t, "another-pack", "1.0.0"), out, "acme", nil, 2); err != nil {
		t.Fatalf("second publish: %v", err)
	}
	idx, err := os.ReadFile(filepath.Join(out, "marketplace.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mp Marketplace
	if err := json.Unmarshal(idx, &mp); err != nil {
		t.Fatalf("the published index is not parseable: %v", err)
	}
	if len(mp.Packs) != 2 {
		t.Fatalf("the catalogue lost a pack: %+v", mp.Packs)
	}
	names := map[string]bool{}
	for _, e := range mp.Packs {
		names[e.Name] = true
	}
	if !names["demo-pack"] || !names["another-pack"] {
		t.Fatalf("the catalogue lost a pack: %+v", mp.Packs)
	}
}

// TestACorruptIndexIsRefused: a corrupt index must NOT be treated as an empty
// catalogue. The old code ignored the parse error and overwrote the index
// "fresh" — which silently dropped every pack published before it, and the
// operator never learned about it. The publish refuses and names the file, so
// the operator can move it aside (to start a new catalogue) or repair it.
func TestACorruptIndexIsRefused(t *testing.T) {
	out := t.TempDir()
	if err := Publish(publishedPack(t, "demo-pack", "2.1.0"), out, "acme", nil, 1); err != nil {
		t.Fatalf("publish: %v", err)
	}
	idxPath := filepath.Join(out, "marketplace.json")
	corrupt := []byte("{ this is not the catalogue")
	if err := os.WriteFile(idxPath, corrupt, 0o644); err != nil {
		t.Fatal(err)
	}

	err := Publish(publishedPack(t, "another-pack", "1.0.0"), out, "acme", nil, 2)
	if err == nil {
		t.Fatal("a corrupt catalogue was silently overwritten; the published packs were dropped")
	}
	if !strings.Contains(err.Error(), "marketplace.json") {
		t.Fatalf("the refusal should name the index: %v", err)
	}
	// The corrupt file is left as it was — the operator's evidence, and their
	// decision to make.
	after, readErr := os.ReadFile(idxPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(corrupt) {
		t.Fatalf("the corrupt index was rewritten: %q", string(after))
	}
	// The pack published before the corruption is still there.
	if _, statErr := os.Stat(filepath.Join(out, "packs", "demo-pack.json")); statErr != nil {
		t.Fatalf("the published pack is gone: %v", statErr)
	}
}

// TestThePublishedFilesAreWritableByTheirOwner: the published files survive
// across publishes, so a leftover file must not be able to stop the next
// publish. os.WriteFile applies its mode only when it CREATES the file, and
// opening a read-only file for writing FAILS — on POSIX with EACCES, on
// Windows with "Access is denied" — so a leftover read-only index would stop
// the operator from publishing at all.
//
// The atomic writer writes a fresh temp file, chmods it to the requested mode
// and renames it over the target, so the result is always writable by its
// owner — and the write never depends on the old file's mode.
func TestThePublishedFilesAreWritableByTheirOwner(t *testing.T) {
	out := t.TempDir()
	if err := Publish(publishedPack(t, "demo-pack", "2.1.0"), out, "acme", nil, 1); err != nil {
		t.Fatalf("publish: %v", err)
	}
	packPath := filepath.Join(out, "packs", "demo-pack.json")
	idxPath := filepath.Join(out, "marketplace.json")
	// Leftovers from an earlier publish that nobody may write.
	if err := os.Chmod(packPath, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(idxPath, 0o400); err != nil {
		t.Fatal(err)
	}

	if err := Publish(publishedPack(t, "demo-pack", "2.2.0"), out, "acme", nil, 2); err != nil {
		t.Fatalf("a leftover read-only file stopped the publish: %v", err)
	}
	if !ownerWritable(t, packPath) {
		t.Fatal("the published pack is not writable by its owner")
	}
	if !ownerWritable(t, idxPath) {
		t.Fatal("the published index is not writable by its owner")
	}
}

// TestThePublishedFilesAreReadableByOthers: the published directory is meant
// to be SERVED — a web server reads these files, and they are not the
// daemon's private state. So they are published 0644.
//
// The permission BITS are a POSIX property; Windows has no group/other
// distinction, so this check runs where the bits exist.
func TestThePublishedFilesAreReadableByOthers(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("POSIX permission bits")
	}
	out := t.TempDir()
	if err := Publish(publishedPack(t, "demo-pack", "2.1.0"), out, "acme", nil, 1); err != nil {
		t.Fatalf("publish: %v", err)
	}
	for _, path := range []string{
		filepath.Join(out, "packs", "demo-pack.json"),
		filepath.Join(out, "marketplace.json"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode&0o044 == 0 {
			t.Fatalf("the published file %s is not readable by others: %v", path, mode)
		}
	}
}
