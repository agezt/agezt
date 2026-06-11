// SPDX-License-Identifier: MIT

package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundleStore_WriteListRead(t *testing.T) {
	bs, err := OpenBundles(t.TempDir())
	if err != nil {
		t.Fatalf("OpenBundles: %v", err)
	}
	files := map[string][]byte{
		"reference/api.md": []byte("# API\nuse the thing"),
		"scripts/setup.sh": []byte("#!/bin/sh\nnpm i -g cowsay"),
		"SKILL.md":         []byte("---\nname: pdf-fill\n---\nbody"),
	}
	rels, err := bs.Write("PDF Fill", files)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Manifest is sorted, slash form.
	want := []string{"SKILL.md", "reference/api.md", "scripts/setup.sh"}
	if strings.Join(rels, ",") != strings.Join(want, ",") {
		t.Fatalf("manifest = %v, want %v", rels, want)
	}
	// List agrees with Write.
	listed, err := bs.List("PDF Fill")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if strings.Join(listed, ",") != strings.Join(want, ",") {
		t.Fatalf("List = %v, want %v", listed, want)
	}
	// Read returns the exact bytes.
	got, err := bs.Read("PDF Fill", "scripts/setup.sh")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != "#!/bin/sh\nnpm i -g cowsay" {
		t.Fatalf("Read content = %q", got)
	}
	// Dir points under bundles/<slug>; the slug folds name casing/spaces.
	if base := filepath.Base(bs.Dir("PDF Fill")); base != "pdf-fill" {
		t.Fatalf("Dir slug = %q, want pdf-fill", base)
	}
}

func TestBundleStore_ReplaceDropsStale(t *testing.T) {
	bs, _ := OpenBundles(t.TempDir())
	if _, err := bs.Write("s", map[string][]byte{"a.txt": []byte("1"), "b.txt": []byte("2")}); err != nil {
		t.Fatal(err)
	}
	// Re-import with only a.txt — b.txt must be gone (bundle is replaced, not merged).
	if _, err := bs.Write("s", map[string][]byte{"a.txt": []byte("1b")}); err != nil {
		t.Fatal(err)
	}
	listed, _ := bs.List("s")
	if len(listed) != 1 || listed[0] != "a.txt" {
		t.Fatalf("after replace, files = %v, want [a.txt]", listed)
	}
	if _, err := bs.Read("s", "b.txt"); err == nil {
		t.Fatalf("stale b.txt still readable after replace")
	}
}

func TestBundleStore_RejectsEscape(t *testing.T) {
	bs, _ := OpenBundles(t.TempDir())
	for _, bad := range []string{"../evil.sh", "/etc/passwd", "a/../../b", "..\\win"} {
		if _, err := bs.Write("s", map[string][]byte{bad: []byte("x")}); err == nil {
			t.Fatalf("Write accepted escaping path %q", bad)
		}
		if _, err := bs.Read("s", bad); err == nil {
			t.Fatalf("Read accepted escaping path %q", bad)
		}
	}
}

func TestBundleStore_FileSizeCap(t *testing.T) {
	bs, _ := OpenBundles(t.TempDir())
	big := make([]byte, MaxBundleFile+1)
	if _, err := bs.Write("s", map[string][]byte{"big.bin": big}); err == nil {
		t.Fatalf("Write accepted an oversized file")
	}
	// And nothing was committed.
	if dir := bs.Dir("s"); dirExists(dir) {
		t.Fatalf("oversized write left a bundle dir behind")
	}
}

func TestBundleStore_AbsentBundle(t *testing.T) {
	bs, _ := OpenBundles(t.TempDir())
	files, err := bs.List("never-written")
	if err != nil {
		t.Fatalf("List of absent bundle errored: %v", err)
	}
	if files != nil {
		t.Fatalf("List of absent bundle = %v, want nil", files)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"PDF Fill":        "pdf-fill",
		"  diagnose-ci  ": "diagnose-ci",
		"a/b\\c":          "a-b-c",
		"Hello   World":   "hello-world",
		"keep_under":      "keep_under",
		"!!!":             "",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
