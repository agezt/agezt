// SPDX-License-Identifier: MIT

package router

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Command is one CLI subcommand. Name is the primary (e.g. "run"),
// Aliases are alternative spellings (e.g. "-h" for "help"). The Run
// callback receives the args AFTER the subcommand name (so `agt run
// --quiet hello` invokes cmdHelp's Run with ["--quiet", "hello"])
// plus the writer pair for stdout/stderr. Run must return the
// process exit code (0 = success, 2 = usage error, etc).
//
// Help is optional; when nil, a one-line description is printed
// for `-h` / `--help` lookups. HelpLong is for `agt help <cmd>`.
// HelpHandler is the optional per-command -h/--help responder;
// when nil the router falls back to Description (the standard
// "Usage: cmd <args>" line).
type Command struct {
	Name        string
	Aliases     []string
	Description string
	HelpLong    string
	HelpHandler func(args []string, stdout, stderr io.Writer) int
	Run         func(args []string, stdout, stderr io.Writer) int
}

// Registry holds the live set of commands. It is not safe for
// concurrent mutation; the convention is Register at init() time
// (single goroutine) and read-only dispatch afterwards.
type Registry struct {
	cmds map[string]*Command
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{cmds: map[string]*Command{}}
}

// Register adds cmd (and every alias) to r. A duplicate Name
// overwrites the previous entry; a duplicate alias points to the
// newer command. This matches the project's pre-router behavior —
// the existing test suite asserts that "agt version" and "agt
// --version" both resolve, and that the version command wins on
// re-registration.
func (r *Registry) Register(cmd *Command) {
	if cmd == nil || cmd.Name == "" {
		return
	}
	r.cmds[cmd.Name] = cmd
	for _, alias := range cmd.Aliases {
		r.cmds[alias] = cmd
	}
}

// Lookup returns the command for name, or nil if none registered.
// Aliases share the same *Command pointer as the primary name, so
// callers that need to distinguish "was this a primary or an
// alias" can read cmd.Aliases and compare.
func (r *Registry) Lookup(name string) *Command {
	if r == nil {
		return nil
	}
	return r.cmds[name]
}

// All returns one *Command per primary name, alphabetically. Aliases
// are de-duplicated so the returned slice has the same length as
// the number of unique commands, not the number of name+alias
// pairs. Used by the help table builder and the test helper.
func (r *Registry) All() []*Command {
	if r == nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]*Command, 0, len(r.cmds))
	for _, cmd := range r.cmds {
		if seen[cmd.Name] {
			continue
		}
		seen[cmd.Name] = true
		out = append(out, cmd)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Execute is the main dispatcher's hot path. It returns the exit
// code from cmd.Run. nil command → "unknown command" error on
// stderr plus a 2 exit code (the conventional CLI "usage error"
// code). The unknown-command branch appends a Suggest hint when
// there's a likely typo.
func (r *Registry) Execute(name string, args []string, stdout, stderr io.Writer, unknownMsg string) int {
	if r == nil {
		return 2
	}
	cmd := r.Lookup(name)
	if cmd == nil {
		fmt.Fprintf(stderr, "%s", unknownMsg)
		if sug := r.Suggest(name, 3); len(sug) > 0 {
			fmt.Fprintf(stderr, " — did you mean %s?", strings.Join(sug, ", "))
		}
		fmt.Fprintln(stderr)
		return 2
	}
	return cmd.Run(args, stdout, stderr)
}

// Suggest returns up to maxSug names from r that are within edit
// distance `tol` of name (Levenshtein, case-insensitive). The
// returned slice is sorted by ascending distance, then by name for
// stability. An empty name or maxSug<=0 returns nil.
func (r *Registry) Suggest(name string, maxSug int) []string {
	if r == nil || name == "" || maxSug <= 0 {
		return nil
	}
	target := strings.ToLower(name)
	const tol = 2 // empirically: 2 catches the common typos ("recieve", "hedlp")
	type cand struct {
		name string
		dist int
	}
	var cands []cand
	for _, cmd := range r.All() {
		d := levenshtein(target, strings.ToLower(cmd.Name), tol+1)
		if d >= 0 && d <= tol {
			cands = append(cands, cand{cmd.Name, d})
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].dist != cands[j].dist {
			return cands[i].dist < cands[j].dist
		}
		return cands[i].name < cands[j].name
	})
	if len(cands) > maxSug {
		cands = cands[:maxSug]
	}
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.name
	}
	return out
}

// levenshtein returns the edit distance between a and b, with
// early-out: if either prefix disagrees by more than `budget`,
// returns -1 (the caller treats -1 as "not a candidate"). For
// short CLI command names (most are <20 chars) this is much
// faster than the full O(len(a)*len(b)) DP table; the budget
// works as an "exit as soon as we know it's too far" prune.
func levenshtein(a, b string, budget int) int {
	la, lb := len(a), len(b)
	if abs(la-lb) > budget {
		return -1
	}
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		rowMin := curr[0]
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
			if curr[j] < rowMin {
				rowMin = curr[j]
			}
		}
		if rowMin > budget {
			return -1
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
