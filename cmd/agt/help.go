// SPDX-License-Identifier: MIT

// agt help: commandHelp + helpGroup types + printHelp + helpHas + cmdHelp + suggestCommands + editDistance.
// Code extracted from help.go during the Day-109 god-file split.
// Public API unchanged.
package main


import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

func printHelp(w io.Writer) {
	fmt.Fprintf(w, "usage: %s <command> [args...]\n", brand.CLI)
	fmt.Fprintf(w, "       %s help <command>   full usage for one command (also: %s <command> -h)\n", brand.CLI, brand.CLI)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "New here? Run `%s quickstart` — it syncs the catalog, adds a provider\n", brand.CLI)
	fmt.Fprintf(w, "key, and prints the exact command to start the daemon.\n")
	for _, g := range helpGroups() {
		fmt.Fprintf(w, "\n%s:\n", g.title)
		for _, c := range g.commands {
			fmt.Fprintf(w, "  %-12s %s\n", c.name, c.summary)
		}
	}
}

// helpHas reports whether the help table documents the command — the gate for
// the uniform `agt <cmd> -h` interception in main.go.
func helpHas(name string) bool {
	for _, g := range helpGroups() {
		for _, c := range g.commands {
			if c.name == name {
				return true
			}
		}
	}
	return false
}

// cmdHelp implements `agt help [<command>]`: the overview without an argument,
// one command's detail block with one. Reads only the table — it never runs
// the command, so `agt help halt` can't halt anything.
func cmdHelp(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}
	want := strings.TrimSpace(args[0])
	for _, g := range helpGroups() {
		for _, c := range g.commands {
			if c.name != want {
				continue
			}
			fmt.Fprintf(stdout, "%s %s — %s\n\n", brand.CLI, c.name, c.summary)
			for _, d := range c.detail {
				// Lines starting with a space are flag/continuation lines of the
				// usage above them — indent, don't re-prefix with the binary name.
				if strings.HasPrefix(d, " ") {
					fmt.Fprintf(stdout, "     %s\n", d)
				} else {
					fmt.Fprintf(stdout, "  %s %s\n", brand.CLI, d)
				}
			}
			return 0
		}
	}
	fmt.Fprintf(stderr, "%s help: unknown command %q", brand.CLI, want)
	if sug := suggestCommands(want); len(sug) > 0 {
		fmt.Fprintf(stderr, " — did you mean %s?", strings.Join(sug, ", "))
	}
	fmt.Fprintf(stderr, "\n")
	return 2
}

// suggestCommands offers near-misses for an unknown command: prefix/substring
// matches plus a small edit-distance pass (≤2) so "jurnal" finds "journal".
func suggestCommands(typo string) []string {
	typo = strings.ToLower(typo)
	if typo == "" {
		return nil
	}
	var out []string
	for _, g := range helpGroups() {
		for _, c := range g.commands {
			if strings.HasPrefix(c.name, typo) || strings.Contains(c.name, typo) ||
				strings.HasPrefix(typo, c.name) || editDistance(typo, c.name) <= 2 {
				out = append(out, c.name)
			}
		}
	}
	sort.Strings(out)
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}

// editDistance is plain Levenshtein over two short ASCII-ish strings — small
// inputs, no allocation concerns beyond two rows.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, min(cur[j-1]+1, prev[j-1]+cost))
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
