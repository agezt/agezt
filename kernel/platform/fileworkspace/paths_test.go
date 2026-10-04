// SPDX-License-Identifier: MIT

package fileworkspace_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/platform/fileworkspace"
)

func TestRootConfigurationAndCreation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, tc := range []struct {
		name, configured, expected string
	}{
		{"default", "", filepath.Join(home, "agezt", "workspace")},
		{"explicit", filepath.Join(home, "configured"), filepath.Join(home, "configured")},
		{"tilde", "~/nested/workspace", filepath.Join(home, "nested", "workspace")},
		{"home", "~", home},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AGEZT_FILE_ROOT", tc.configured)
			root, err := fileworkspace.Root()
			if err != nil || root != tc.expected {
				t.Fatalf("root=%q error=%v, want %q", root, err, tc.expected)
			}
			info, err := os.Stat(root)
			if err != nil || !info.IsDir() {
				t.Fatalf("root was not created: info=%v error=%v", info, err)
			}
		})
	}
}

func TestResolveRetainsPathAndErrorContract(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AGEZT_FILE_ROOT", root)
	for _, tc := range []struct {
		input, relative string
	}{
		{"", "."},
		{".", "."},
		{" dir//new.txt ", "dir/new.txt"},
		{"does/not/exist/yet.txt", "does/not/exist/yet.txt"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			gotRoot, target, rel, err := fileworkspace.Resolve(tc.input)
			if err != nil || gotRoot != root || rel != tc.relative || target != filepath.Join(root, filepath.FromSlash(tc.relative)) {
				t.Fatalf("root=%q target=%q relative=%q error=%v", gotRoot, target, rel, err)
			}
		})
	}
	for _, input := range []string{"../escape", "x/../../escape", "/absolute", `C:\absolute`, `\absolute`, "bad\x00path"} {
		if _, _, _, err := fileworkspace.Resolve(input); err == nil {
			t.Errorf("accepted invalid path %q", input)
		}
	}
	if err := fileworkspace.VerifyResolvedWithinRoot(root, filepath.Join(root, "absent", "new.txt")); err != nil {
		t.Fatalf("missing in-root tail refused: %v", err)
	}
	if err := fileworkspace.VerifyResolvedWithinRoot(root, t.TempDir()); err == nil {
		t.Fatal("accepted out-of-root existing path")
	}
}

func TestSanitizeRetainsExactErrors(t *testing.T) {
	for _, tc := range []struct{ input, message string }{
		{"bad\x00path", "path contains NUL"},
		{"/absolute", "path must be relative"},
		{`C:\absolute`, "path must be relative"},
		{`\absolute`, "path must be relative"},
		{"../escape", "path contains '..'"},
	} {
		if _, err := fileworkspace.SanitizeRelativePath(tc.input); err == nil || err.Error() != tc.message {
			t.Errorf("input=%q error=%v, want %q", tc.input, err, tc.message)
		}
	}
}
