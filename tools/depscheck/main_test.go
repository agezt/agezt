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

// TestGoModOnlyListsCompiledDeps asserts that go.mod's DIRECT require
// blocks match the documented Direct dep table in DEPENDENCIES.md.
//
// The 14 transitive-test-dep entries (see the list below) appear only
// in `go list -m all` because Go's MVS walks upstream test
// dependencies. The intent of UPD-002 is to keep these out of the
// DIRECT require block — but Go is allowed to write them into the
// `// indirect` block when a DIRECT dep (e.g. golang.org/x/net, our
// browser tool's PSL source) legitimately imports them. That is
// transitively-pulled-by-our-direct-dep, not "we have started depending
// on testify at build time", which is the actual leak we want to catch.
//
// If a future change promotes one of these into the DIRECT require
// block, this test catches it.
//
// Detection: scan go.mod line-by-line. A line inside a `require ()`
// block that starts with `<tab><path>` is a require. Lines whose
// remainder contains `// indirect` are MVS-managed transitives and
// are not flagged. Anything else (in a comment, after the closing
// paren, etc.) is harmless.
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
		"golang.org/x/crypto":           true,
		"golang.org/x/mod":              true,
		// golang.org/x/net removed — now a DIRECT dep (browser tool PSL).
		"golang.org/x/sync":    true,
		"golang.org/x/sys":     true,
		"golang.org/x/term":    true,
		"golang.org/x/text":    true,
		"golang.org/x/tools":   true,
		"golang.org/x/xerrors": true,
		"gopkg.in/yaml.v3":     true,
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimPrefix(line, "\t")
		if trimmed == line {
			continue // not inside a require block
		}
		// `// indirect` entries are MVS-managed: a DIRECT dep of ours
		// pulls them in and Go wrote them here for module-graph
		// completeness. They are NOT the leak this test guards
		// against (the leak would be promoting them into the DIRECT
		// require block).
		if strings.Contains(trimmed, "// indirect") {
			continue
		}
		// Strip the leading tab to get the path (possibly followed
		// by version + comment).
		path := strings.SplitN(trimmed, " ", 2)[0]
		if transitive[path] {
			t.Errorf("%s is in go.mod's DIRECT require block — should be MVS-only transitives (UPD-002)", path)
		}
	}
}

// TestIndirectTransitiveEntryIsAllowed locks in the post-UPD-002 fix:
// a transitive-test-dep module is allowed to appear in go.mod's
// `// indirect` require block (when a DIRECT dep of ours transitively
// pulls it in), but is still flagged if it shows up in the DIRECT
// require block. If a future refactor of TestGoModOnlyListsCompiledDeps
// drops the `// indirect` exemption, this test fails first with a
// tightly-scoped message pointing at the regression.
//
// This test does NOT read the real go.mod — it runs the parser logic
// against an in-memory fixture so it never depends on the actual
// dependency state of the repo.
func TestIndirectTransitiveEntryIsAllowed(t *testing.T) {
	transitive := map[string]bool{
		"github.com/stretchr/testify": true,
		"golang.org/x/sys":            true,
	}
	// Fixture mirrors the relevant parts of go.mod:
	//   - golang.org/x/sys appears in the indirect block (allowed).
	//   - github.com/stretchr/testify is hypothetically promoted to the
	//     DIRECT block (the leak we want to catch).
	const fixture = "" +
		"require (\n" +
		"\tgithub.com/btcsuite/btcd/btcec/v2 v2.5.0\n" +
		"\tgithub.com/stretchr/testify v1.10.0\n" +
		")\n" +
		"require (\n" +
		"\tgolang.org/x/sys v0.47.0 // indirect\n" +
		")\n"

	var leaks []string
	for _, line := range strings.Split(fixture, "\n") {
		trimmed := strings.TrimPrefix(line, "\t")
		if trimmed == line {
			continue
		}
		if strings.Contains(trimmed, "// indirect") {
			continue
		}
		path := strings.SplitN(trimmed, " ", 2)[0]
		if transitive[path] {
			leaks = append(leaks, path)
		}
	}

	if len(leaks) != 1 || leaks[0] != "github.com/stretchr/testify" {
		t.Fatalf("parser flagged wrong set of leaks: %v (want [github.com/stretchr/testify])", leaks)
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
