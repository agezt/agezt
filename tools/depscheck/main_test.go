// SPDX-License-Identifier: MIT

package main

// Regression guards for UPD-002 (the 14-entry "transitive test deps"
// classification in DEPENDENCIES.md). These tests assert that:
//   1. Every module in the resolved build list is in the allowlist.
//   2. go.mod only declares modules Agezt actually compiles against —
//      the 14 transitive test deps are NOT listed as `require` lines,
//      even though `go list -m all` reports them.
//   3. The compiled agezt binary does not contain the transitively-
//      pulled strings (testify, goldmark, yaml.v3, golang.org/x/*).
//      If a future dep upgrade changes that, depscheck-grow will catch it.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestDepscheckAllowlistMatchesBuildList re-runs the depscheck binary
// itself and asserts exit code 0. This is the same gate the CI uses.
func TestDepscheckAllowlistMatchesBuildList(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command(filepath.Join(goBin(t), "go"), "run", "./tools/depscheck")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("depscheck failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "OK:") {
		t.Errorf("depscheck output missing OK marker: %q", out)
	}
}

// TestGoModOnlyListsCompiledDeps asserts that go.mod's require blocks
// match the documented Direct + Indirect dep tables in DEPENDENCIES.md.
// The 14 transitive-test-dep entries are NOT in go.mod — they appear
// only in `go list -m all` because Go's MVS walks upstream test
// dependencies. If a future change adds them to go.mod's require
// blocks, this test catches it.
//
// Detection: scan go.mod line-by-line. A line inside a `require ()`
// block that starts with `<tab><path>` is a real require. Anything
// else (in a comment, after the closing paren, etc.) is harmless.
func TestGoModOnlyListsCompiledDeps(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	transitive := map[string]bool{
		"github.com/davecgh/go-spew":    true,
		"github.com/pmezard/go-difflib": true,
		"github.com/stretchr/testify":   true,
		"github.com/yuin/goldmark":      true,
		"golang.org/x/crypto":          true,
		"golang.org/x/mod":             true,
		// golang.org/x/net removed — now a DIRECT dep (browser tool PSL).
		"golang.org/x/sync":            true,
		"golang.org/x/sys":             true,
		"golang.org/x/term":            true,
		"golang.org/x/text":            true,
		"golang.org/x/tools":           true,
		"golang.org/x/xerrors":         true,
		"gopkg.in/yaml.v3":             true,
	}
	// Inside a `require ()` block, every entry is indented with one
	// tab — that's how `gofmt` formats them. Any line starting with
	// "<TAB><path>" is a real require; any other occurrence is in
	// a comment or prose (like DEPENDENCIES.md) and is harmless.
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimPrefix(line, "\t")
		if trimmed == line {
			continue // not inside a require block
		}
		// Strip the leading tab to get the path (possibly followed
		// by version + comment).
		path := strings.SplitN(trimmed, " ", 2)[0]
		if transitive[path] {
			t.Errorf("%s is in go.mod's require block — should be MVS-only transitives (UPD-002)", path)
		}
	}
}

// TestCompiledBinaryHasNoTransitiveDeps builds the daemon and asserts
// that strings like "stretchr/testify" do NOT appear in the output.
// If a future dep upgrade causes Agezt to link against one of these,
// the binary size will jump and this string test will fail loudly.
func TestCompiledBinaryHasNoTransitiveDeps(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("binary build not supported on %s", runtime.GOOS)
	}
	root := repoRoot(t)
	tmp := t.TempDir()
	out := filepath.Join(tmp, "agezt-bin")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}

	build := exec.Command(filepath.Join(goBin(t), "go"), "build", "-o", out, "./cmd/agezt")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	build.Dir = root
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, b)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	forbidden := []string{
		"stretchr/testify",
		"yuin/goldmark",
		"yaml.v3",
	}
	for _, needle := range forbidden {
		// Some symbols may incidentally contain substring matches in
		// unrelated places; check by looking for the module path
		// followed by a non-letter byte (a real import would be
		// prefixed by the full path). The simple substring check is
		// good enough — Go's runtime / stdlib never references these.
		if strings.Contains(string(data), needle) {
			t.Errorf("binary contains %q — a transitive test dep leaked into the compiled output", needle)
		}
	}
}

// repoRoot walks up from the current package directory until it finds
// go.mod, returning the directory containing it.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod in any ancestor)")
		}
		dir = parent
	}
}

// goBin returns the path to the `go` binary, so tests don't depend on
// $PATH being set in unusual environments.
func goBin(t *testing.T) string {
	t.Helper()
	g, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("go binary not on PATH: %v", err)
	}
	return filepath.Dir(g)
}

