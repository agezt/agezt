// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The "frontend test files" probes are deliberately narrow, and that is the
// fragile part. The audit report quotes three vitest file counts: 159 in the
// where-it-started gate table, 161 mid-report, and 162 for the current state.
// Only the last is a claim about the tip. A probe loose enough to catch all three
// would fail the gate on two correct descriptions of a past state, and the usual
// response to a gate that cries wolf is to switch it off — so the scoping is
// pinned here rather than left to a future reader's judgement.
func TestFrontendTestFilesProbeCatchesOnlyTheCurrentState(t *testing.T) {
	probes := claimPatterns["frontend test files"]
	if len(probes) == 0 {
		t.Fatal(`no probe registered for "frontend test files"`)
	}

	cases := []struct {
		name    string
		line    string
		capture string
		want    bool
	}{
		{"current state, audit report", "| vitest | 162 files / 1464 tests, 0 failures |", "162", true},
		{"current state, PR body", "`go test ./...` 192 packages · vitest 162 files / 1464 tests ·", "162", true},
		{"historical gate table", "| 9 | `frontend-test` | `vitest` | ✅ 159 files |", "", false},
		{"historical mid-report", "`tsc --noEmit` ✅ · vitest **161 files / 1449 tests, 0 failures** ✅", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ""
			for _, p := range probes {
				re := regexp.MustCompile(p)
				if m := re.FindStringSubmatch(c.line); len(m) >= 2 {
					got = m[1]
					break
				}
			}
			if c.want && got != c.capture {
				t.Fatalf("probe did not capture %q from %q (got %q); it would let the claim drift unchecked", c.capture, c.line, got)
			}
			if !c.want && got != "" {
				t.Fatalf("probe captured %q from %q; a correct description of a past state would be reported as drift", got, c.line)
			}
		})
	}
}

// countTestFiles must skip the directories that would otherwise inflate the
// count: node_modules carries a copy of the test tree for any dependency that
// ships one, and dist/coverage are build output. Only frontend/src is counted,
// so a stray test outside it is a different question.
func TestCountTestFilesSkipsGeneratedTrees(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/src"
	mustWrite(t, src+"/a.test.ts", "")
	mustWrite(t, src+"/b.test.tsx", "")
	mustWrite(t, src+"/c.ts", "")      // not a test
	mustWrite(t, src+"/d.test.js", "") // not a .ts/.tsx test
	mustWrite(t, src+"/node_modules/x.test.ts", "")
	mustWrite(t, src+"/dist/y.test.ts", "")
	mustWrite(t, src+"/coverage/z.test.ts", "")

	if got, want := countTestFiles(dir, "src"), "2"; got != want {
		t.Errorf("countTestFiles = %q, want %q", got, want)
	}
}

// A missing tree must report "unmeasured", not zero. main() skips a fact whose
// measurement is empty; a hard "0" would instead be compared against the
// documents and reported as drift, which is the wrong diagnosis.
func TestCountTestFilesReportsNothingForAMissingTree(t *testing.T) {
	if got := countTestFiles(t.TempDir(), "no-such-dir"); got != "" {
		t.Errorf("countTestFiles on a missing tree = %q, want \"\" (unmeasured)", got)
	}
}

// Every registered fact must have a probe, and every probe must name a fact that
// exists. A probe with no fact is dead code that looks like coverage; a fact
// with no probe is a measurement nothing checks, which is the exact failure this
// tool was written to end.
func TestClaimPatternsAndFactsCorrespond(t *testing.T) {
	for label := range claimPatterns {
		if _, ok := measuredLabels[label]; !ok {
			t.Errorf("probe registered for %q, which no fact measures", label)
		}
	}
	for label := range measuredLabels {
		if len(claimPatterns[label]) == 0 {
			t.Errorf("fact %q is measured but no document claim is checked against it", label)
		}
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if i := strings.LastIndex(path, "/"); i >= 0 {
		if err := os.MkdirAll(path[:i], 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path[:i], err)
		}
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
