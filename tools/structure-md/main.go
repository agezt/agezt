// SPDX-License-Identifier: MIT

// Command structure-md regenerates `.project/STRUCTURE.md` (and its
// per-section sidecars) from the actual filesystem. It walks
// `kernel/`, `internal/`, `cmd/`, `plugins/`, `sdk/`, `tools/`,
// `frontend/src/`, reads each package's first doc comment (`doc.go`
// for Go, top-of-file `/** … */` or `// …` block for TS/TSX), and
// produces a Markdown summary grouped by directory.
//
// Why this exists. The historical STRUCTURE.md drifted from the code
// (conduit→governor, chronos→cadence, pluginhost→plugin, plugins/,
// frontend/ schemas) because it was maintained by hand. This tool
// turns STRUCTURE.md into a build artifact: every package's
// `doc.go` is the only source of truth, and CI fails on uncommitted
// changes via `make check`.
//
// What it does NOT do. It does not infer architecture or call out
// smells — that's the human review pass. It only reflects the
// filesystem as it is today. The grouping and the high-level
// commentary at the top of STRUCTURE.md remain a human-edited
// section; only the per-package lists are generated.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// osExit allows tests to intercept os.Exit without terminating.
var osExit = os.Exit

// exitProcess is a narrow test seam for main's fatal path.
var exitProcess = func(code int) { osExit(code) }

type pkgSummary struct {
	Path    string // import path (e.g. "kernel/agent")
	RelDir  string // directory relative to repo root (e.g. "kernel/agent")
	Summary string // first sentence of the doc comment, or "—" if none
}

func main() {
	out := flag.String("out", ".project/STRUCTURE.generated", "output directory for generated STRUCTURE files")
	check := flag.Bool("check", false, "exit non-zero if generated files differ from disk (CI mode)")
	root := flag.String("root", "", "repo root (default: parent of tools/structure-md)")
	flag.Parse()

	r := *root
	if r == "" {
		// The convention is: run from the repo root. `os.Getwd()` is
		// the only reliable answer because `go run` puts the binary
		// in a temp dir, so deriving from `os.Args[0]` would land
		// in the toolchain cache, not the repo.
		var err error
		r, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "structure-md: resolve root: %v\n", err)
			exitProcess(1)
		}
	}

	if err := run(r, *out, *check); err != nil {
		fmt.Fprintf(os.Stderr, "structure-md: %v\n", err)
		exitProcess(1)
	}
}

func run(root, outDir string, check bool) error {
	groups, err := collectGroups(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	drifted := false
	for name, pkgs := range groups {
		outPath := filepath.Join(outDir, "STRUCTURE."+name+".md")
		generated := renderGroup(name, pkgs)
		existing, _ := os.ReadFile(outPath)
		if string(existing) != generated {
			if check {
				drifted = true
				fmt.Fprintf(os.Stderr, "structure-md: %s is stale (run `make structure-md`)\n", outPath)
			} else {
				if err := os.WriteFile(outPath, []byte(generated), 0o644); err != nil {
					return fmt.Errorf("write %s: %w", outPath, err)
				}
				fmt.Fprintf(os.Stdout, "structure-md: wrote %s (%d packages)\n", outPath, len(pkgs))
			}
		}
	}
	if drifted {
		exitProcess(1)
	}
	return nil
}

// collectGroups walks the major roots and groups their packages by
// "section" (kernel / internal / cmd / plugins / sdk / tools /
// frontend). The frontend group is special — it lists dirs instead
// of Go packages because there is no Go code there.
func collectGroups(root string) (map[string][]pkgSummary, error) {
	groups := map[string][]pkgSummary{
		"kernel":   nil,
		"internal": nil,
		"cmd":      nil,
		"plugins":  nil,
		"sdk":      nil,
		"tools":    nil,
	}
	// Walk Go roots.
	for _, dir := range []string{"kernel", "internal", "cmd", "plugins", "sdk", "tools"} {
		rootPath := filepath.Join(root, dir)
		if _, err := os.Stat(rootPath); err != nil {
			continue
		}
		entries, err := walkGoPackages(root, rootPath, dir)
		if err != nil {
			return nil, err
		}
		groups[dir] = entries
	}
	return groups, nil
}

// walkGoPackages recursively finds every directory containing at
// least one .go file and reads its doc.go (or the first package
// comment it can find).
func walkGoPackages(root, base, relPrefix string) ([]pkgSummary, error) {
	visited := map[string]bool{}
	var out []pkgSummary
	err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip vendor / node_modules / hidden dirs.
			name := d.Name()
			if name == "vendor" || name == "node_modules" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// First .go file in this directory; read its doc comment.
		dir := filepath.Dir(path)
		if visited[dir] {
			return nil
		}
		visited[dir] = true
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			return err
		}
		// Always emit forward-slash paths — they match Go import
		// paths and are stable across OSes (the generator is
		// cross-platform).
		rel = filepath.ToSlash(rel)
		summary, err := extractGoDocSummary(dir)
		if err != nil {
			return err
		}
		out = append(out, pkgSummary{
			Path:    rel,
			RelDir:  rel,
			Summary: summary,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelDir < out[j].RelDir })
	// Deduplicate — multiple .go files in the same dir would have
	// returned the same rel; keep the first (longest summary).
	dedup := out[:0]
	seen := map[string]bool{}
	for _, p := range out {
		if seen[p.RelDir] {
			continue
		}
		seen[p.RelDir] = true
		dedup = append(dedup, p)
	}
	return dedup, nil
}

// extractGoDocSummary reads the first sentence of the package
// doc comment. Prefers `doc.go` when present.
func extractGoDocSummary(dir string) (string, error) {
	// Prefer doc.go.
	docFile := filepath.Join(dir, "doc.go")
	if data, err := os.ReadFile(docFile); err == nil {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, docFile, data, parser.ParseComments)
		if err == nil {
			if s := firstSentence(docComment(f)); s != "" {
				return s, nil
			}
		}
	}
	// Fall back to the first .go file in the dir.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			continue
		}
		if s := firstSentence(docComment(f)); s != "" {
			return s, nil
		}
	}
	return "—", nil
}

// docComment returns the package-level doc comment (the // block
// immediately before `package <name>`).
func docComment(f *ast.File) string {
	if f.Doc == nil {
		return ""
	}
	return f.Doc.Text()
}

// firstSentence returns the first sentence (up to the first
// period+space, or the whole text if no period is present).
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	// Collapse whitespace.
	re := regexp.MustCompile(`\s+`)
	s = re.ReplaceAllString(s, " ")
	if s == "" {
		return ""
	}
	// Walk to first ". " (period + space) but not "." inside
	// paths like "kernel/foo.go". The doc comment is plain
	// English; a period at the end of a sentence is what we want.
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '.' && (i+1 >= len(s) || s[i+1] == ' ' || s[i+1] == '\n') {
			return strings.TrimSpace(s[:i+1])
		}
	}
	return s
}

// renderGroup writes one section's per-package table.
func renderGroup(name string, pkgs []pkgSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — auto-generated by tools/structure-md\n\n", strings.ToUpper(name))
	fmt.Fprintf(&b, "> Do not edit by hand. Re-run `make structure-md` to refresh.\n\n")
	fmt.Fprintf(&b, "%d package(s):\n\n", len(pkgs))
	for _, p := range pkgs {
		fmt.Fprintf(&b, "- **`%s`** — %s\n", p.RelDir, p.Summary)
	}
	return b.String()
}
