// SPDX-License-Identifier: MIT

// Command deadcodecheck runs the Go deadcode analyzer and fails on new
// repository-local unreachable code. The public Go SDK is allowlisted because
// repository-local reachability cannot see external SDK consumers.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const deadcodeVersion = "v0.47.0"

// osExit allows tests to intercept os.Exit without terminating.
var osExit = os.Exit

// newDeadcodeCmd is injectable in tests to avoid actually running the analyzer.
var newDeadcodeCmd = func() *exec.Cmd {
	return exec.Command("go", "run", "golang.org/x/tools/cmd/deadcode@"+deadcodeVersion, "./...")
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
	var allowed int
	for _, line := range lines {
		if isAllowedSDKFinding(line) {
			allowed++
			continue
		}
		unexpected = append(unexpected, line)
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
	if allowed > 0 {
		fmt.Fprintf(stdout, "OK: no unexpected dead code; %d public SDK findings allowlisted.\n", allowed)
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

func isAllowedSDKFinding(line string) bool {
	normalized := strings.ReplaceAll(line, `\`, "/")
	return strings.HasPrefix(normalized, "sdk/") &&
		strings.Contains(normalized, ": unreachable func:")
}
