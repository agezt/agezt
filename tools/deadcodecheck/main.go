// SPDX-License-Identifier: MIT

// Command deadcodecheck runs the Go deadcode analyzer and fails on new
// repository-local unreachable code. The public Go SDK is allowlisted because
// repository-local reachability cannot see external SDK consumers.
//
// The analyzer runs WITHOUT -test on purpose, so "reachable" means "reachable
// from a binary". That strictness is the point: it is what surfaced
// Set.NetguardGaps — a drift alarm that looked healthy because its own unit
// test called it, while boot never did. Adding -test would clear most findings
// in one line and permanently hide that class. The narrow escape hatch for
// symbols a test in another package must reach is testOnlyCrossPackageSeams.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const deadcodeVersion = "v0.47.0"

// osExit allows tests to intercept os.Exit without terminating.
var osExit = os.Exit

// goBinary resolves the `go` executable of the toolchain this binary was built
// with, falling back to PATH lookup. On the WSL CI runners the Windows PATH is
// appended to the Linux one, and a stray npm shim named `go` there shadowed the
// real toolchain ("exec: node: not found") — pinning to runtime.GOROOT() makes
// the subprocess immune to PATH ordering.
func goBinary() string {
	//lint:ignore SA1019 deadcodecheck always runs in-repo via `go run`, never as
	// a copied binary — the building toolchain's GOROOT is exactly the pin we want.
	if root := runtime.GOROOT(); root != "" {
		bin := filepath.Join(root, "bin", "go")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
	}
	return "go"
}

// newDeadcodeCmd is injectable in tests to avoid actually running the analyzer.
var newDeadcodeCmd = func() *exec.Cmd {
	return exec.Command(goBinary(), "run", "golang.org/x/tools/cmd/deadcode@"+deadcodeVersion, "./...")
}

func main() {
	cmd := newDeadcodeCmd()
	out, err := cmd.CombinedOutput()
	osExit(runChecker(os.Stdout, os.Stderr, out, err))
}

// runChecker runs the deadcode analyzer and returns the exit code and stdout.
// Used by tests to avoid calling os.Exit.
func runChecker(stdout io.Writer, stderr io.Writer, cmdOut []byte, cmdErr error) int {
	lines := findingLines(cmdOut)
	var unexpected []string
	var allowed, seams int
	for _, line := range lines {
		switch {
		case isAllowedSDKFinding(line):
			allowed++
		case isAllowedTestOnlySeam(line):
			seams++
		default:
			unexpected = append(unexpected, line)
		}
	}

	if len(unexpected) > 0 {
		fmt.Fprintln(stderr, "deadcodecheck: unexpected dead code findings:")
		for _, line := range unexpected {
			fmt.Fprintln(stderr, "  "+line)
		}
		return 1
	}
	if cmdErr != nil {
		fmt.Fprintf(stderr, "deadcodecheck: analyzer failed: %v\n%s", cmdErr, cmdOut)
		return 1
	}
	if allowed > 0 || seams > 0 {
		fmt.Fprintf(stdout, "OK: no unexpected dead code; %d public SDK findings and %d cross-package test seams allowlisted.\n", allowed, seams)
		return 0
	}
	fmt.Fprintln(stdout, "OK: no dead code findings.")
	return 0
}

func findingLines(out []byte) []string {
	var lines []string
	for _, raw := range bytes.Split(out, []byte{'\n'}) {
		line := strings.TrimSpace(string(raw))
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// testOnlyCrossPackageSeams are symbols with no binary caller whose ONLY
// consumer is a test in a DIFFERENT package. Those cannot be moved into a
// _test.go beside their user — a test can only reach another package through
// its exported API — so they have to stay in production files and the
// analyzer will always report them.
//
// This is deliberately narrow. A same-package test-only helper does NOT belong
// here: move it into a _test.go in its own package instead, which states what
// it is and keeps the gate strict. Three were moved that way on 2026-08-12
// (channelwire.Kinds, toolreg.Lookup, the governor budget probes) and only one
// symbol genuinely could not be.
//
// Keyed by "file|symbol" rather than a directory prefix on purpose: a package
// prefix would also hide the NEXT dead function in the same package, which is
// how a gate quietly stops gating.
var testOnlyCrossPackageSeams = map[string]bool{
	// toolreg.Names pins the exact ordered boot tool surface for
	// plugins/builtintools/ratchet_test.go, so the tool list cannot change
	// silently. toolreg cannot import the package that registers the specs
	// (that is the dependency direction), so the seam stays exported.
	"kernel/toolreg/toolreg.go|Names": true,
}

// isAllowedTestOnlySeam reports whether a finding is a pinned cross-package
// test seam. The finding shape is "path:line:col: unreachable func: Symbol".
func isAllowedTestOnlySeam(line string) bool {
	const marker = ": unreachable func: "
	normalized := strings.ReplaceAll(line, `\`, "/")
	i := strings.Index(normalized, marker)
	if i < 0 {
		return false
	}
	symbol := normalized[i+len(marker):]
	file := normalized[:i]
	if j := strings.Index(file, ":"); j >= 0 {
		file = file[:j]
	}
	return testOnlyCrossPackageSeams[file+"|"+symbol]
}

func isAllowedSDKFinding(line string) bool {
	normalized := strings.ReplaceAll(line, `\`, "/")
	return (strings.HasPrefix(normalized, "sdk/") &&
		strings.Contains(normalized, ": unreachable func:")) ||
		(strings.HasPrefix(normalized, "kernel/internal/testfixtures/") &&
			strings.Contains(normalized, ": unreachable func:")) ||
		(strings.HasPrefix(normalized, "kernel/delegation/") &&
			strings.Contains(normalized, ": unreachable func:")) ||
		(strings.HasPrefix(normalized, "kernel/workflowexec/") &&
			strings.Contains(normalized, ": unreachable func:")) ||
		(strings.HasPrefix(normalized, "kernel/journal/runs.go") &&
			strings.Contains(normalized, ": unreachable func:")) ||
		// acpcatalog exports ResolveLaunch for external SDK consumers
		// (IDE ACP agent launch resolution); deadcodecheck can't see them.
		(strings.HasPrefix(normalized, "kernel/acpcatalog/") &&
			strings.Contains(normalized, ": unreachable func:"))
}
