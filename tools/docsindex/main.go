// SPDX-License-Identifier: MIT

// Command docsindex checks the documentation index's own invariants.
//
// docs/index.md is the public map of the docs/ tree, and it drifts in three
// ways that are all invisible until somebody follows a link that is not
// there:
//
//   - a row points at a file that was renamed, moved or archived away;
//   - a document in docs/ is not listed at all, so it is invisible but still
//     in the tree;
//   - an ARCHIVED document is still listed as a live one.
//
// The third one is why this gate exists. The banner alone does not archive
// anything, and the index alone does not remember it — four documents sat in
// the tree with an ARCHIVED banner and were still listed as current. The two
// have to agree, and this gate is what makes them agree.
//
// The rules:
//
//  1. every local link in the index points at a file that exists;
//  2. every .md file in docs/ is listed in the index at least once;
//  3. a document that carries an ARCHIVED banner is listed only under the
//     archive section, and a document without one is listed only outside it.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ArchiveSection is the section that holds the archived documents.
const ArchiveSection = "Archived documents"

// linkRE matches a markdown link and captures its target.
var linkRE = regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)

// archivedRE matches the banner that marks a document as archived.
var archivedRE = regexp.MustCompile(`(?m)^> \*\*Status:\*\* ARCHIVED`)

// entry is one document's place in the index.
type entry struct {
	// Name is the file's name inside docs/.
	Name string
	// Section is the "## " heading it is listed under.
	Section string
}

// Check reports every invariant the index breaks.
//
// docsDir is the documentation directory; indexPath is the index file inside
// it. The returned errors are human-readable and name the file at fault.
func Check(docsDir, indexPath string) []error {
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		return []error{fmt.Errorf("read the index: %w", err)}
	}
	index := string(raw)

	var errs []error

	// The sections and the documents listed in them.
	entries, links := parse(index)
	listed := map[string][]string{}
	for _, e := range entries {
		listed[e.Name] = append(listed[e.Name], e.Section)
	}

	// Rule 1: every local link points at something that exists.
	for _, l := range links {
		target := l.Target
		if strings.Contains(target, "#") {
			target = target[:strings.Index(target, "#")]
		}
		if target == "" {
			continue
		}
		resolved := filepath.Join(docsDir, filepath.FromSlash(target))
		if _, err := os.Stat(resolved); err != nil {
			errs = append(errs, fmt.Errorf("the index links %q, which does not exist", l.Target))
		}
	}

	// Rule 2: every document in docs/ is listed at least once.
	docs, err := os.ReadDir(docsDir)
	if err != nil {
		return append(errs, fmt.Errorf("read the docs directory: %w", err))
	}
	var names []string
	for _, d := range docs {
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			continue
		}
		if d.Name() == filepath.Base(indexPath) {
			continue // the index does not list itself
		}
		names = append(names, d.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		if len(listed[name]) == 0 {
			errs = append(errs, fmt.Errorf("%s is in docs/ but the index never lists it", name))
		}
	}

	// Rule 3: the banner and the section agree.
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(docsDir, name))
		if err != nil {
			errs = append(errs, fmt.Errorf("read %s: %w", name, err))
			continue
		}
		archived := archivedRE.Match(body)
		for _, section := range listed[name] {
			inArchive := section == ArchiveSection
			if archived && !inArchive {
				errs = append(errs, fmt.Errorf("%s carries an ARCHIVED banner but is listed under %q", name, section))
			}
			if !archived && inArchive {
				errs = append(errs, fmt.Errorf("%s is listed under %q but carries no ARCHIVED banner", name, section))
			}
		}
	}

	return errs
}

// link is one local link in the index.
type link struct {
	Target string
}

// parse reads the index's sections and its local links.
func parse(index string) ([]entry, []link) {
	var entries []entry
	var links []link
	section := ""
	seen := map[string]bool{}

	for _, line := range strings.Split(index, "\n") {
		if strings.HasPrefix(line, "## ") {
			section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
		}
		for _, m := range linkRE.FindAllStringSubmatch(line, -1) {
			target := strings.TrimSpace(m[1])
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			links = append(links, link{Target: target})
			name := filepath.Base(filepath.FromSlash(target))
			if name == "" || name == "." || seen[name+"|"+section] {
				continue
			}
			seen[name+"|"+section] = true
			entries = append(entries, entry{Name: name, Section: section})
		}
	}
	return entries, links
}

func main() {
	index := flag.String("index", filepath.Join("docs", "index.md"), "the documentation index")
	flag.Parse()

	docsDir := filepath.Dir(*index)
	errs := Check(docsDir, *index)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "docsindex: %v\n", e)
		}
		fmt.Fprintf(os.Stderr, "docsindex: %d problem(s) in %s\n", len(errs), *index)
		os.Exit(1)
	}
	fmt.Printf("OK: the documentation index agrees with the docs/ tree.\n")
}
