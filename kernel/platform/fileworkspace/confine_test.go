// SPDX-License-Identifier: MIT

package fileworkspace

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestConfineUnder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	for rel, want := range map[string]string{
		"demo":          filepath.Join(root, "demo"),
		" demo ":        filepath.Join(root, "demo"),
		"demo/src/a.go": filepath.Join(root, "demo", "src", "a.go"),
		"demo/../other": filepath.Join(root, "other"),
		"./demo":        filepath.Join(root, "demo"),
		".":             root,
		"demo/..":       root,
	} {
		if got, ok := ConfineUnder(root, rel); !ok || got != want {
			t.Fatalf("%q: got %q ok=%v, want %q", rel, got, ok, want)
		}
	}
	// "../projects-x/a" lands in a sibling whose name only starts with the root's.
	bad := []string{"", "   ", "..", "../x", "demo/../../x", "../projects-x/a", "a\x00b", filepath.Join(root, "abs")}
	if runtime.GOOS == "windows" {
		bad = append(bad, `C:\Windows`, `\\server\share`)
	} else {
		bad = append(bad, "/etc/passwd")
	}
	for _, rel := range bad {
		if got, ok := ConfineUnder(root, rel); ok || got != "" {
			t.Fatalf("%q escaped: %q", rel, got)
		}
	}
	if _, ok := ConfineUnder(root+"-sibling", "x"); !ok {
		t.Fatal("a sibling root is its own root")
	}
	if got, ok := ConfineUnder(root, "..projects-x"); !ok || got != filepath.Join(root, "..projects-x") {
		t.Fatal("a name that only starts with dots stays inside", got, ok)
	}
}
