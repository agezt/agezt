// SPDX-License-Identifier: MIT

package fileworkspace_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/platform/fileworkspace"
)

func TestMutationPrimitivesRetainConsoleSemantics(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "parent", "child")
	if err := fileworkspace.Mkdir(deep, false); !os.IsNotExist(err) {
		t.Fatalf("mkdir without parents: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(deep)); !os.IsNotExist(err) {
		t.Fatalf("nonrecursive mkdir created its parent: %v", err)
	}
	if err := fileworkspace.Mkdir(deep, true); err != nil {
		t.Fatal(err)
	}
	if err := fileworkspace.Mkdir(deep, false); !os.IsExist(err) {
		t.Fatalf("mkdir existing directory: %v", err)
	}
	source := filepath.Join(deep, "notes.txt")
	if err := os.WriteFile(source, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "renamed.txt")
	if err := fileworkspace.Rename(source, destination); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(destination); err != nil || string(content) != "content" {
		t.Fatalf("rename changed content: %q error=%v", content, err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("rename retained source: %v", err)
	}
	if err := fileworkspace.Delete(filepath.Dir(deep), false); err == nil {
		t.Fatal("nonrecursive delete removed a nonempty directory")
	}
	if _, err := os.Stat(deep); err != nil {
		t.Fatalf("failed nonrecursive delete changed subtree: %v", err)
	}
	if err := fileworkspace.Delete(filepath.Dir(deep), true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(deep); !os.IsNotExist(err) {
		t.Fatalf("recursive delete retained subtree: %v", err)
	}
	if err := fileworkspace.Delete(destination, true); err != nil {
		t.Fatal(err)
	}
	if err := fileworkspace.Delete(destination, false); !os.IsNotExist(err) {
		t.Fatalf("missing-file OS error lost: %v", err)
	}
}

func TestDeleteRefusesFinalSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "original.txt")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("file symlink unavailable on this host: %v", err)
	}
	for _, recursive := range []bool{false, true} {
		if err := fileworkspace.Delete(link, recursive); !errors.Is(err, fileworkspace.ErrSymlink) {
			t.Fatalf("recursive=%v symlink deletion: %v", recursive, err)
		}
		if _, err := os.Lstat(link); err != nil {
			t.Fatalf("refused deletion removed link: %v", err)
		}
		if content, err := os.ReadFile(target); err != nil || string(content) != "keep" {
			t.Fatalf("refused deletion changed target: %q error=%v", content, err)
		}
	}
}
