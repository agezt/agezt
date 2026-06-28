// SPDX-License-Identifier: MIT

// Command deadcodecheck runs the Go deadcode analyzer and fails on new
// repository-local unreachable code. The public Go SDK is allowlisted because
// repository-local reachability cannot see external SDK consumers.
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const deadcodeVersion = "v0.47.0"

func main() {
	cmd := exec.Command("go", "run", "golang.org/x/tools/cmd/deadcode@"+deadcodeVersion, "./...")
	out, err := cmd.CombinedOutput()
	lines := findingLines(out)

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
		fmt.Fprintln(os.Stderr, "deadcodecheck: unexpected dead code findings:")
		for _, line := range unexpected {
			fmt.Fprintln(os.Stderr, "  "+line)
		}
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "deadcodecheck: analyzer failed: %v\n%s", err, out)
		os.Exit(1)
	}
	if allowed > 0 {
		fmt.Fprintf(os.Stdout, "OK: no unexpected dead code; %d public SDK findings allowlisted.\n", allowed)
		return
	}
	fmt.Fprintln(os.Stdout, "OK: no dead code findings.")
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
