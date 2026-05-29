// SPDX-License-Identifier: MIT

// Command depscheck verifies that every Go module pulled into the build is
// listed in tools/depscheck/allowlist.txt. POLICY §1.1 requires every
// external dependency in the core to be justified in DEPENDENCIES.md; the
// allowlist is the machine-checkable mirror of that table.
//
// Exit codes:
//
//	0 — every required module is in the allowlist
//	1 — at least one required module is absent (or another error occurred)
//
// The check uses `go list -m -f '{{if not .Main}}{{.Path}}{{end}}' all`,
// which is the canonical way to enumerate the build's resolved module graph.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

func main() {
	allowed, err := readAllowlist("tools/depscheck/allowlist.txt")
	if err != nil {
		fail("read allowlist: %v", err)
	}

	required, err := listModules()
	if err != nil {
		fail("list modules: %v", err)
	}

	var unjustified []string
	for _, m := range required {
		if !allowed[m] {
			unjustified = append(unjustified, m)
		}
	}
	if len(unjustified) > 0 {
		sort.Strings(unjustified)
		fmt.Fprintln(os.Stderr, "ERROR: unjustified core dependencies:")
		for _, m := range unjustified {
			fmt.Fprintln(os.Stderr, "  -", m)
		}
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Add each to DEPENDENCIES.md with a justification, then to")
		fmt.Fprintln(os.Stderr, "tools/depscheck/allowlist.txt. See POLICY §1.")
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "OK: %d core dependencies, all justified.\n", len(required))
}

func readAllowlist(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[string]bool)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = true
	}
	return out, sc.Err()
}

func listModules() ([]string, error) {
	cmd := exec.Command("go", "list", "-m", "-f", "{{if not .Main}}{{.Path}}{{end}}", "all")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w (stderr: %s)", err, stderr(err))
	}
	var mods []string
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			mods = append(mods, line)
		}
	}
	return mods, nil
}

func stderr(err error) string {
	if ee, ok := err.(*exec.ExitError); ok {
		return string(ee.Stderr)
	}
	return ""
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "depscheck: "+format+"\n", args...)
	os.Exit(1)
}
