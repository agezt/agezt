// SPDX-License-Identifier: MIT

package app_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The tree names no UNTYPED handle.
//
// An `any` in a handle's place is a door: anything can walk through it, and
// the compiler will not say what. The adapter's doors closed first (W5.5c:
// the constructors take a scope, by type), and the scope's door closed with
// them (W5.5e: the application layer's bridge fills the families' ports from
// the daemon's own kernel, by type).
//
// The rule is about the SHAPE of the parameter: a parameter that stands for
// the daemon's kernel and whose TYPE is `any`. There are TWO shapes to that
// door, and the second one is the reason this rule exists:
//
//   - a parameter whose NAME is a handle's name and whose TYPE is `any`
//     (`handle any`, `k any`, `kernel any`)
//   - a family fill that takes an `any` handle at all — a function named
//     `...FromHandle` whose parameters hold an `any`
//
// The first shape alone would have left six fills open: they all named their
// handle `h`, and a rule that only knew the handle's LONG names would not
// have seen them. A JSON payload (`map[string]any`) is not a handle, and a
// family that decodes into an `any` is not a handle either — only a value
// that stands for the daemon's kernel.
var untypedHandleParameter = regexp.MustCompile(
	`\b(?:k|kh|kernel|handle|h)\s+any\b` + // a handle's name, typed as any
		`|\bfunc\s+\w*FromHandle\([^)]*\bany\b`) // a family fill that takes an any handle

// untypedHandleViolations are the production files that name an untyped
// handle.
//
// The list is sorted: a map's order is random, and a report whose order
// changes between two runs of the same code is a report that cannot be
// compared between two runs.
func untypedHandleViolations(sources map[string]string) []string {
	var violations []string
	for name, src := range sources {
		if untypedHandleParameter.MatchString(src) {
			violations = append(violations, name)
		}
	}
	sort.Strings(violations)
	return violations
}

// TestTheKernelNamesNoUntypedHandle holds the count: the kernel names an
// untyped handle in exactly the recorded number of files.
func TestTheKernelNamesNoUntypedHandle(t *testing.T) {
	sources := map[string]string{}
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sources[filepath.ToSlash(path)] = string(raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) == 0 {
		t.Fatal("no kernel sources found — the tree moved?")
	}
	const want = 0
	violations := untypedHandleViolations(sources)
	if len(violations) != want {
		t.Errorf("the kernel names an untyped handle in %d files (%s) — the recorded count is %d",
			len(violations), strings.Join(violations, ", "), want)
	}
}

// The rule is held by the rule: a matcher that could not see the violation
// would let the handle walk back in through the very door this slice closed,
// and nothing else in the tree would notice.
//
// The fixtures carry all THREE shapes the door has worn: the long handle
// name, the short one the rule used to miss, and the family fill that takes
// an `any` handle under a name the rule does not recognise at all.
func TestTheUntypedHandleRuleSeesTheViolationItExistsFor(t *testing.T) {
	sources := map[string]string{
		"violating-named.go":   "func Fill[T any](handle any, fill func(*runtime.Kernel) T) T {\n\treturn fill(handle.(*runtime.Kernel))\n}\n",
		"violating-short.go":   "func Fill[T any](h any, fill func(*runtime.Kernel) T) T {\n\treturn fill(h.(*runtime.Kernel))\n}\n",
		"violating-fill.go":    "func PortsFromHandle(h any, baseDir string) Ports {\n\treturn Ports{}\n}\n",
		"clean.go":             "func Fill[T any](k *runtime.Kernel, fill func(*runtime.Kernel) T) T {\n\treturn fill(k)\n}\n",
		"clean-payload.go":     "func decode(raw json.RawMessage) any {\n\tvar v any\n\t_ = json.Unmarshal(raw, &v)\n\treturn v\n}\n",
		"clean-map-payload.go": "func payload() map[string]any {\n\treturn map[string]any{}\n}\n",
	}
	violations := untypedHandleViolations(sources)
	want := []string{"violating-fill.go", "violating-named.go", "violating-short.go"}
	if len(violations) != len(want) {
		t.Fatalf("the rule saw %v, want %v", violations, want)
	}
	for i, name := range want {
		if violations[i] != name {
			t.Fatalf("the rule saw %v, want %v", violations, want)
		}
	}
}
