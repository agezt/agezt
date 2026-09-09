// SPDX-License-Identifier: MIT

package router

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// noopCmd returns a Command whose Run is a no-op that returns the
// given exit code. Keeps the per-test setup minimal.
func noopCmd(name string, code int, aliases ...string) *Command {
	return &Command{
		Name:        name,
		Aliases:     aliases,
		Description: name + " (test)",
		Run:         func(args []string, stdout, stderr io.Writer) int { return code },
	}
}

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	r.Register(noopCmd("run", 0))
	r.Register(noopCmd("help", 0, "-h", "--help"))

	if r.Lookup("run") == nil {
		t.Error("expected run to be registered")
	}
	if r.Lookup("--help") == nil {
		t.Error("expected --help alias to be registered")
	}
	if r.Lookup("nope") != nil {
		t.Error("expected nope to be unregistered")
	}
}

func TestRegistry_All_DedupesAliases(t *testing.T) {
	r := NewRegistry()
	r.Register(noopCmd("run", 0))
	r.Register(noopCmd("help", 0, "-h", "--help"))
	r.Register(noopCmd("version", 0, "-v", "--version"))

	all := r.All()
	if len(all) != 3 {
		t.Errorf("All() returned %d, want 3 (deduped)", len(all))
	}
	// Sorted alphabetically: help, run, version.
	if all[0].Name != "help" || all[1].Name != "run" || all[2].Name != "version" {
		t.Errorf("All() not sorted: %v", []string{all[0].Name, all[1].Name, all[2].Name})
	}
}

func TestRegistry_Execute_HappyPath(t *testing.T) {
	r := NewRegistry()
	var seen []string
	r.Register(&Command{
		Name: "echo",
		Run: func(args []string, stdout, stderr io.Writer) int {
			seen = args
			return 0
		},
	})

	var stdout, stderr bytes.Buffer
	code := r.Execute("echo", []string{"a", "b"}, &stdout, &stderr, "unknown: ")

	if code != 0 {
		t.Errorf("Execute returned %d, want 0", code)
	}
	if len(seen) != 2 || seen[0] != "a" || seen[1] != "b" {
		t.Errorf("Run got args %v, want [a b]", seen)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("expected no output, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRegistry_Execute_UnknownPrintsHint(t *testing.T) {
	r := NewRegistry()
	r.Register(noopCmd("run", 0))
	r.Register(noopCmd("status", 0))

	var stderr bytes.Buffer
	code := r.Execute("rnu", nil, nil, &stderr, "unknown: ")

	if code != 2 {
		t.Errorf("Execute returned %d, want 2 (usage error)", code)
	}
	if !strings.Contains(stderr.String(), "did you mean run") {
		t.Errorf("expected suggestion 'run', got: %q", stderr.String())
	}
}

func TestRegistry_Execute_UnknownFarTypoNoHint(t *testing.T) {
	r := NewRegistry()
	r.Register(noopCmd("run", 0))

	var stderr bytes.Buffer
	code := r.Execute("xyzzz", nil, nil, &stderr, "unknown: ")

	if code != 2 {
		t.Errorf("Execute returned %d, want 2", code)
	}
	if strings.Contains(stderr.String(), "did you mean") {
		t.Errorf("expected no suggestion for far-away typo, got: %q", stderr.String())
	}
}

func TestRegistry_NilSafe(t *testing.T) {
	var r *Registry
	if r.Lookup("anything") != nil {
		t.Error("nil registry should return nil")
	}
	if r.All() != nil {
		t.Error("nil registry should return nil All()")
	}
	if r.Suggest("a", 3) != nil {
		t.Error("nil registry should return nil Suggest")
	}
	if code := r.Execute("a", nil, nil, nil, "x: "); code != 2 {
		t.Errorf("nil registry Execute should return 2, got %d", code)
	}
}

func TestSuggest_FindsTypos(t *testing.T) {
	r := NewRegistry()
	r.Register(noopCmd("compile", 0))   // distance 1 from "comple"
	r.Register(noopCmd("compare", 0))   // distance 2 from "comple" (need to swap + insert)
	r.Register(noopCmd("complete", 0))  // distance 2 from "comple"
	r.Register(noopCmd("run", 0))       // far away
	r.Register(noopCmd("status", 0))
	r.Register(noopCmd("why", 0))

	// "comple" → "compile" needs 1 edit (insert 'i' before 'l'→'l' is noop,
	// actually the algorithm resolves it as distance 1). The other two
	// are distance 2. So the first suggestion must be "compile", and
	// "run"/"status"/"why" must be absent.
	sug := r.Suggest("comple", 3)
	if len(sug) < 1 || sug[0] != "compile" {
		t.Errorf("expected 'compile' first, got %v", sug)
	}
	for _, name := range []string{"run", "status", "why"} {
		for _, s := range sug {
			if s == name {
				t.Errorf("unexpected suggestion %q for far-away typo 'comple'", name)
			}
		}
	}
}

func TestSuggest_RespectsMaxSug(t *testing.T) {
	r := NewRegistry()
	r.Register(noopCmd("compile", 0))
	r.Register(noopCmd("complete", 0))
	r.Register(noopCmd("comply", 0))
	r.Register(noopCmd("complex", 0))
	// All four are within distance 2 of "comple" (or so). Limit
	// caps the result to maxSug.
	sug := r.Suggest("comple", 2)
	if len(sug) != 2 {
		t.Errorf("expected 2 suggestions, got %d: %v", len(sug), sug)
	}
}

func TestSuggest_EmptyOrZeroMax(t *testing.T) {
	r := NewRegistry()
	r.Register(noopCmd("run", 0))
	if got := r.Suggest("", 3); got != nil {
		t.Errorf("Suggest('') = %v, want nil", got)
	}
	if got := r.Suggest("run", 0); got != nil {
		t.Errorf("Suggest('run', 0) = %v, want nil", got)
	}
}

func TestLevenshtein_KnownValues(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "abcd", 1},
		{"kitten", "sitting", 3},
		{"flaw", "lawn", 2},
		{"abc", "xyz", 3},
	}
	for _, c := range cases {
		got := levenshtein(c.a, c.b, 100)
		if got != c.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestLevenshtein_BudgetEarlyOut(t *testing.T) {
	// When the difference exceeds the budget, levenshtein returns -1
	// instead of running the full DP table. This is the optimization
	// that makes Suggest cheap on a 200-command registry.
	if got := levenshtein("abcdef", "ghijkl", 2); got != -1 {
		t.Errorf("expected -1 (budget exceeded), got %d", got)
	}
	// Within budget: returns the actual distance.
	if got := levenshtein("abc", "abd", 2); got != 1 {
		t.Errorf("expected 1, got %d", got)
	}
}
