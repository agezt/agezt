// SPDX-License-Identifier: MIT

package skill

// Provenance: the bundle store's publish guarantee (W5.4) — a write that
//             fails must leave the published bundle exactly as it was, and
//             must not leave its staging tree behind.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/internal/dirswap"
)

// TestBundleStoreKeepsThePreviousBundleWhenTheWriteFails: the write fails
// while the staging tree is being filled, and the published bundle survives.
//
// The old code deleted the bundle FIRST and then renamed the staged one over
// it, so a failure in between left the skill with NO bundle at all — the
// content was deleted, not moved, and could not be recovered.
func TestBundleStoreKeepsThePreviousBundleWhenTheWriteFails(t *testing.T) {
	bs, err := OpenBundles(t.TempDir())
	if err != nil {
		t.Fatalf("OpenBundles: %v", err)
	}
	// A bundle with a file in it, so the next write can be compared with it.
	if _, err := bs.Write("s", map[string][]byte{"keep.txt": []byte("first")}); err != nil {
		t.Fatalf("the first write: %v", err)
	}

	// "a" cannot be a file while "a" is a directory: whichever of the two is
	// written first, the other cannot be written. The write therefore fails
	// AFTER the staging tree was prepared — on every platform and whatever
	// the order the map is walked in.
	_, err = bs.Write("s", map[string][]byte{
		"a":   []byte("x"),
		"a/b": []byte("y"),
	})
	if err == nil {
		t.Fatal("the write of a file and a directory with the same name must fail")
	}

	got, err := bs.Read("s", "keep.txt")
	if err != nil {
		t.Fatalf("the previous bundle is gone: %v", err)
	}
	if string(got) != "first" {
		t.Fatalf("the bundle holds %q, want the published %q", got, "first")
	}

	// The staging tree must not be left behind: it is the only place the
	// failed attempt's half-written files could survive.
	staging := filepath.Join(filepath.Dir(bs.Dir("s")), "s"+dirswap.StagingSuffix)
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Fatalf("the staging tree was left behind at %s (stat err = %v)", staging, err)
	}
}

// TestBundleStorePublishesTheWholeBundle: a successful write publishes the
// bundle as a whole — every file is there, and the tree it replaced is gone.
func TestBundleStorePublishesTheWholeBundle(t *testing.T) {
	bs, err := OpenBundles(t.TempDir())
	if err != nil {
		t.Fatalf("OpenBundles: %v", err)
	}
	files := map[string][]byte{
		"SKILL.md":         []byte("---\nname: x\n---\n"),
		"reference/api.md": []byte("# API"),
	}
	rels, err := bs.Write("s", files)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if len(rels) != 2 {
		t.Fatalf("the manifest holds %v, want two entries", rels)
	}
	for rel, want := range files {
		got, err := bs.Read("s", rel)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if string(got) != string(want) {
			t.Fatalf("%s holds %q, want %q", rel, got, want)
		}
	}
}
