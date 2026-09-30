// SPDX-License-Identifier: MIT

// Command docclaimscheck verifies the quantitative claims in the audit's own
// deliverables against the branch they describe.
//
// The 2026-09-27/30 surface audit produced three documents under .project/ that
// state hard numbers: files changed, commit count, doc.go count, changelog
// entry count. Four separate passes found those numbers stale, each time
// because a document was written while the work was still in progress and
// never re-measured:
//
//   - a review map quoting 1068 files and "twelve commits" while the branch
//     carried 20, and counting a denominator (1,372) measured after 60 of the
//     79 files under investigation already existed on disk
//   - a PR body claiming every CI job had been run, when 12 of 17 had
//   - a changelog count of 156 in a document whose own two later commits added
//     two more entries, making it 158
//
// The pattern is not carelessness in any one place. It is that documentation
// written during work drifts silently, and nothing fails when it does. This
// tool is the thing that fails.
//
// It reads the claims out of the documents and the facts out of git, and
// exits non-zero on any mismatch. It does not check prose and does not judge
// whether a claim is meaningful — only whether a number the document states is
// still the number.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var osExit = os.Exit

type doc struct {
	path string
}

var (
	flagBase  = flag.String("base", "origin/main", "ref the documents claim to describe")
	flagProj  = flag.String("dir", ".project", "directory holding the deliverable documents")
	flagQuiet = flag.Bool("quiet", false, "only report failures")
)

var documents = []string{
	"REVIEW-MAP.md",
	"AUDIT-2026-09-BACKEND-SURFACE.md",
	"PR-BODY.md",
}

type fact struct {
	label  string
	actual string
}

func main() {
	flag.Parse()

	base := *flagBase
	docs := make([]doc, 0, len(documents))
	for _, n := range documents {
		docs = append(docs, doc{path: *flagProj + "/" + n})
	}

	facts := []fact{
		{"commits on the branch", gitField(`rev-list --count `+base+`..HEAD`, `(\d+)`)},
		{"files changed", gitField(`diff --shortstat `+base+`..HEAD`, `(\d+) files? changed`)},
		{"files modified", gitField(`diff --name-only --diff-filter=M `+base+`..HEAD`, `(?s).*`, true)},
		{"files added", gitField(`diff --name-only --diff-filter=A `+base+`..HEAD`, `(?s).*`, true)},
		{"files deleted", gitField(`diff --name-only --diff-filter=D `+base+`..HEAD`, `(?s).*`, true)},
		{"files renamed", gitField(`diff -M --name-only --diff-filter=R `+base+`..HEAD`, `(?s).*`, true)},
		{"doc.go files", countAddedMatching(base+"..HEAD", `doc\.go$`)},
		{"changelog entries", countEntries("CHANGELOG/unreleased/current.md")},
		{"CI jobs", countYAMLJobs(".github/workflows/ci.yml")},
	}

	fails := 0
	for _, f := range facts {
		if f.actual == "" {
			fmt.Printf("  %-22s could not be measured (skipped)\n", f.label)
			continue
		}
		if !*flagQuiet {
			fmt.Printf("  %-22s = %s\n", f.label, f.actual)
		}
		fails += checkClaims(docs, f)
	}

	if fails > 0 {
		fmt.Fprintf(os.Stderr, "\ndocclaimscheck: %d claim(s) no longer match the branch. "+
			"A document written while the work was in progress has drifted.\n", fails)
		osExit(1)
	}
	fmt.Println("OK: the deliverables' numbers match the branch.")
}

// checkClaims looks for each fact's value in the documents and reports any
// number adjacent to a phrase that should have matched it.
func checkClaims(docs []doc, f fact) int {
	fails := 0
	for _, d := range docs {
		b, err := os.ReadFile(d.path)
		if err != nil {
			continue
		}
		text := string(b)
		for _, probe := range claimPatterns[f.label] {
			re := regexp.MustCompile(probe)
			for _, loc := range re.FindAllStringSubmatchIndex(text, -1) {
				stated := text[loc[2]:loc[3]]
				if stated == f.actual {
					continue
				}
				line := 1 + strings.Count(text[:loc[0]], "\n")
				fmt.Fprintf(os.Stderr, "  %s:%d states %q for %s, but it is %s\n",
					d.path, line, stated, f.label, f.actual)
				fails++
			}
		}
	}
	return fails
}

// claimPatterns ties a fact to the phrasings the documents use for it.
// claimPatterns ties a fact to the phrasings the documents use for it.
//
// Two things about these that are easy to get wrong, and both of them make the
// gate pass while checking nothing:
//
//   - they need (?m). In Go, ^ and $ anchor to the whole text, not to a line,
//     so a pattern like ^## The (\d+) commits$ silently matches nothing in a
//     multi-line document. A negative test (plant a wrong number, expect a
//     failure) is what catches this; the happy path looks identical either way.
//   - they need \r? before $. The documents are CRLF on Windows, and a bare $
//     will not match "…package comments\r".
var claimPatterns = map[string][]string{
	"commits on the branch": {
		`(?m)^## The (\d+) commits\r?$`,
		`carries \*\*(\d+)\*\* commits`,
		`(\d+) commits ahead of main`,
	},
	"files changed": {
		`(?m)^(\d+) files changed\r?$`,
	},
	"files modified":    {`(\d+) modified`},
	"files added":       {`(\d+) added`},
	"files deleted":     {`(\d+) deleted`},
	"files renamed":     {`(\d+) renamed`},
	"doc.go files":      {`(?m)^## \d+\. Restore (\d+) package comments\r?$`},
	"changelog entries": {`\*\*(\d+) entries\*\*`},
	"CI jobs":           {`defines \*\*(\d+) jobs\*\*`},
}

func git(args ...string) ([]byte, error) {
	return exec.Command("git", args...).Output()
}

// gitField runs `git <args>` and extracts a number: the first capture group of
// `pattern`, or a line count when `count` is set.
func gitField(args, pattern string, count ...bool) string {
	out, err := git(strings.Fields(args)...)
	if err != nil {
		return ""
	}
	text := string(out)
	if len(count) > 0 && count[0] {
		n := 0
		for _, l := range strings.Split(text, "\n") {
			if strings.TrimSpace(l) != "" {
				n++
			}
		}
		return strconv.Itoa(n)
	}
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(strings.TrimSpace(text))
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// countAddedMatching counts files the branch ADDS whose path matches. The
// claim being checked is "this audit added 80 package comments", so the
// measurement has to be of the additions, not of what exists on a ref.
func countAddedMatching(rangeSpec, pattern string) string {
	out, err := git("diff", "--name-only", "--diff-filter=A", rangeSpec)
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(pattern)
	n := 0
	for _, l := range strings.Split(string(out), "\n") {
		l = strings.TrimSpace(l)
		if l != "" && re.MatchString(l) {
			n++
		}
	}
	return strconv.Itoa(n)
}

func countEntries(file string) string {
	b, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	return strconv.Itoa(len(regexp.MustCompile(`(?m)^- \*\*`).FindAllString(string(b), -1)))
}

func countYAMLJobs(file string) string {
	b, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(b), "\n")
	start := -1
	for i, l := range lines {
		// CRLF files keep the \r through strings.Split("\n")
		if strings.TrimRight(l, " \t\r") == "jobs:" {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return ""
	}
	job := regexp.MustCompile(`^  ([A-Za-z0-9_-]+):\r?\s*$`)
	n := 0
	for _, l := range lines[start:] {
		// In a CRLF file a blank line survives strings.Split("\n") as "\r",
		// so test for whitespace-only rather than for "".
		if strings.TrimSpace(l) == "" {
			continue
		}
		// a new top-level key ends the jobs block
		if !strings.HasPrefix(l, " ") && !strings.HasPrefix(l, "\t") {
			break
		}
		if job.MatchString(l) {
			n++
		}
	}
	return strconv.Itoa(n)
}
