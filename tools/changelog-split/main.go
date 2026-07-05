// SPDX-License-Identifier: MIT

// Command changelog-split parses CHANGELOG.md and materializes the planned
// split structure under CHANGELOG/. It is intentionally conservative:
// - released versions are copied as-is into per-version files,
// - the Unreleased block is split by `###` subsection,
// - subsections with M###/M#### references are bucketed into 100-wide files,
// - subsections with no M reference stay in `unreleased/current.md`.
//
// The tool supports three modes:
//
//	--dry-run   print the planned output map only (default when no write flag is set)
//	--emit      write the generated files under --out-dir
//	--verify    compare the generated files to --out-dir and fail on drift
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var mRefRe = regexp.MustCompile(`\bM(\d{3,4})\b`)
var versionHeaderRe = regexp.MustCompile(`^## \[([^\]]+)\]\s*[—-]\s*(\d{4}-\d{2}-\d{2})\s*$`)

type versionBlock struct {
	Header string
	Tag    string
	Date   string
	Body   []string
}

type unreleasedChunk struct {
	Header string
	Body   []string
	Bucket string // current | m100-m199 | m1000+
}

type splitResult struct {
	Readme        string
	ReorgLog      string
	Current       string
	Buckets       map[string]string // path -> content
	Released      map[string]string // path -> content
	MainChangelog string
}

func main() {
	source := flag.String("source", "CHANGELOG.md", "source changelog file")
	outDir := flag.String("out-dir", "CHANGELOG", "output directory for split files")
	mainOut := flag.String("main-out", "", "path to write the rewritten root changelog when emitting (default: overwrite --source)")
	dryRun := flag.Bool("dry-run", false, "print the planned outputs without writing")
	emit := flag.Bool("emit", false, "write the generated files")
	verify := flag.Bool("verify", false, "compare generated files to --out-dir")
	flag.Parse()

	if !*dryRun && !*emit && !*verify {
		*dryRun = true
	}

	b, err := os.ReadFile(*source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "changelog-split: read %s: %v\n", *source, err)
		os.Exit(1)
	}
	res, err := buildSplit(string(b))
	if err != nil {
		fmt.Fprintf(os.Stderr, "changelog-split: %v\n", err)
		os.Exit(1)
	}

	if *dryRun {
		printPlan(res)
	}
	if *emit {
		rootTarget := *mainOut
		if strings.TrimSpace(rootTarget) == "" {
			rootTarget = *source
		}
		if err := writeResult(rootTarget, *outDir, res); err != nil {
			fmt.Fprintf(os.Stderr, "changelog-split: emit: %v\n", err)
			os.Exit(1)
		}
	}
	if *verify {
		if err := verifyResult(*outDir, res); err != nil {
			fmt.Fprintf(os.Stderr, "changelog-split: verify: %v\n", err)
			os.Exit(1)
		}
	}
}

func buildSplit(src string) (splitResult, error) {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	unreleasedIdx := -1
	var versions []versionBlock
	for i, line := range lines {
		if line == "## [Unreleased]" {
			unreleasedIdx = i
			continue
		}
		if m := versionHeaderRe.FindStringSubmatch(line); m != nil {
			versions = append(versions, versionBlock{Header: line, Tag: m[1], Date: m[2]})
		}
	}
	if unreleasedIdx < 0 {
		return splitResult{}, errors.New("missing `## [Unreleased]` header")
	}
	if len(versions) == 0 {
		return splitResult{}, errors.New("no released version blocks found")
	}

	// Find body ranges for released blocks.
	versionPositions := make([]int, 0, len(versions))
	for i, line := range lines {
		if versionHeaderRe.MatchString(line) {
			versionPositions = append(versionPositions, i)
		}
	}
	for i := range versions {
		start := versionPositions[i] + 1
		end := len(lines)
		if i+1 < len(versionPositions) {
			end = versionPositions[i+1]
		}
		versions[i].Body = trimTrailingBlankLines(lines[start:end])
	}

	unreleasedEnd := versionPositions[0]
	unreleasedBody := trimTrailingBlankLines(lines[unreleasedIdx+1 : unreleasedEnd])
	chunks := splitUnreleased(unreleasedBody)

	buckets := map[string][]unreleasedChunk{}
	for _, c := range chunks {
		buckets[c.Bucket] = append(buckets[c.Bucket], c)
	}

	res := splitResult{
		Readme:        renderReadme(versions, buckets),
		ReorgLog:      renderReorgLog(versions, buckets),
		Current:       renderBucketDoc("current", buckets["current"]),
		Buckets:       map[string]string{},
		Released:      map[string]string{},
		MainChangelog: renderMain(lines[:unreleasedIdx], versions),
	}
	for bucket, items := range buckets {
		if bucket == "current" {
			continue
		}
		res.Buckets[filepath.ToSlash(filepath.Join("unreleased", bucket+".md"))] = renderBucketDoc(bucket, items)
	}
	for _, v := range versions {
		res.Released[vFilename(v)] = renderVersion(v)
	}
	return res, nil
}

func splitUnreleased(body []string) []unreleasedChunk {
	var out []unreleasedChunk
	var cur *unreleasedChunk
	flush := func() {
		if cur == nil {
			return
		}
		cur.Body = trimTrailingBlankLines(cur.Body)
		cur.Bucket = bucketFor(cur.Header, cur.Body)
		out = append(out, *cur)
	}
	for _, line := range body {
		if strings.HasPrefix(line, "### ") {
			flush()
			cur = &unreleasedChunk{Header: line}
			continue
		}
		if cur == nil {
			cur = &unreleasedChunk{Header: "### Unclassified"}
		}
		cur.Body = append(cur.Body, line)
	}
	flush()
	return out
}

func bucketFor(header string, body []string) string {
	all := header + "\n" + strings.Join(body, "\n")
	matches := mRefRe.FindAllStringSubmatch(all, -1)
	if len(matches) == 0 {
		return "current"
	}
	min := 999999
	for _, m := range matches {
		var n int
		fmt.Sscanf(m[1], "%d", &n)
		if n < min {
			min = n
		}
	}
	if min >= 1000 {
		return "m1000+"
	}
	// The historical M600-M699 slice is much denser than its neighbors in the
	// current changelog. Split only that range into 50-wide buckets so the tree
	// stays readable without exploding the number of files elsewhere.
	if min >= 600 && min < 700 {
		start := (min / 50) * 50
		if start < 600 {
			start = 600
		}
		return fmt.Sprintf("m%d-m%d", start, start+49)
	}
	start := (min / 100) * 100
	if start < 100 {
		start = 100
	}
	return fmt.Sprintf("m%d-m%d", start, start+99)
}

func renderBucketDoc(name string, chunks []unreleasedChunk) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Changelog — %s\n\n", name)
	if name == "current" {
		b.WriteString("This file holds the active `[Unreleased]` working set.\n\n")
	} else {
		b.WriteString("Historical slices extracted from the old `[Unreleased]` block.\n\n")
	}
	for i, c := range chunks {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(c.Header)
		b.WriteString("\n")
		if len(c.Body) > 0 {
			b.WriteString(strings.Join(c.Body, "\n"))
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderVersion(v versionBlock) string {
	var b strings.Builder
	b.WriteString("# Changelog\n\n")
	b.WriteString(v.Header)
	b.WriteString("\n\n")
	if len(v.Body) > 0 {
		b.WriteString(strings.Join(v.Body, "\n"))
		b.WriteString("\n")
	}
	return b.String()
}

func vFilename(v versionBlock) string {
	return fmt.Sprintf("v%s.md", v.Tag)
}

func renderMain(prefix []string, versions []versionBlock) string {
	var b strings.Builder
	b.WriteString(strings.Join(trimTrailingBlankLines(prefix), "\n"))
	b.WriteString("\n\n## [Unreleased]\n\n")
	b.WriteString("See `CHANGELOG/unreleased/current.md` for the active working set and `CHANGELOG/` for historical milestone slices.\n\n")
	b.WriteString("## Releases\n\n")
	b.WriteString("Released version notes live in per-version files under `CHANGELOG/`.\n\n")
	for _, v := range versions {
		fmt.Fprintf(&b, "- `%s` — `%s` (%s)\n", vFilename(v), v.Tag, v.Date)
	}
	return b.String()
}

func renderReadme(versions []versionBlock, buckets map[string][]unreleasedChunk) string {
	var b strings.Builder
	b.WriteString("# Changelog\n\n")
	b.WriteString("This directory holds the split changelog structure.\n\n")
	b.WriteString("## Layout\n\n")
	b.WriteString("- `unreleased/current.md` — active working set\n")
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		if k != "current" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "- `unreleased/%s.md` — historical milestone slice\n", k)
	}
	for _, v := range versions {
		fmt.Fprintf(&b, "- `%s` — released version %s\n", vFilename(v), v.Tag)
	}
	b.WriteString("- `REORG-LOG.md` — reorg history\n")
	return b.String()
}

func renderReorgLog(versions []versionBlock, buckets map[string][]unreleasedChunk) string {
	var b strings.Builder
	b.WriteString("# Changelog Reorg Log\n\n")
	b.WriteString("Generated by `tools/changelog-split`.\n\n")
	b.WriteString("## Released versions\n\n")
	for _, v := range versions {
		fmt.Fprintf(&b, "- `%s` (%s)\n", v.Tag, v.Date)
	}
	b.WriteString("\n## Unreleased slices\n\n")
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "- `%s` — %d subsection(s)\n", k, len(buckets[k]))
	}
	return b.String()
}

func writeResult(mainPath, outDir string, res splitResult) error {
	if err := os.MkdirAll(filepath.Join(outDir, "unreleased"), 0o755); err != nil {
		return err
	}
	writes := map[string]string{
		mainPath:                                          res.MainChangelog,
		filepath.Join(outDir, "README.md"):                res.Readme,
		filepath.Join(outDir, "REORG-LOG.md"):             res.ReorgLog,
		filepath.Join(outDir, "unreleased", "current.md"): res.Current,
	}
	for path, content := range res.Buckets {
		writes[filepath.Join(outDir, filepath.FromSlash(path))] = content
	}
	for path, content := range res.Released {
		writes[filepath.Join(outDir, filepath.FromSlash(path))] = content
	}
	if err := removeStaleSplitFiles(outDir, writes); err != nil {
		return err
	}
	for path, content := range writes {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func removeStaleSplitFiles(outDir string, writes map[string]string) error {
	root := filepath.Clean(outDir)
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		if _, keep := writes[path]; keep {
			return nil
		}
		return os.Remove(path)
	})
}

func verifyResult(outDir string, res splitResult) error {
	expected := map[string]string{
		filepath.Join(outDir, "README.md"):                res.Readme,
		filepath.Join(outDir, "REORG-LOG.md"):             res.ReorgLog,
		filepath.Join(outDir, "unreleased", "current.md"): res.Current,
	}
	for path, content := range res.Buckets {
		expected[filepath.Join(outDir, filepath.FromSlash(path))] = content
	}
	for path, content := range res.Released {
		expected[filepath.Join(outDir, filepath.FromSlash(path))] = content
	}
	var bad []string
	for path, want := range expected {
		got, err := os.ReadFile(path)
		if err != nil {
			bad = append(bad, fmt.Sprintf("missing %s", path))
			continue
		}
		if !bytes.Equal(got, []byte(want)) {
			bad = append(bad, fmt.Sprintf("drift %s", path))
		}
	}
	if len(bad) > 0 {
		return errors.New(strings.Join(bad, "; "))
	}
	return nil
}

func printPlan(res splitResult) {
	fmt.Println("changelog-split dry-run")
	fmt.Printf("  main: rewritten `CHANGELOG.md` with compact Unreleased pointer\n")
	fmt.Printf("  current: CHANGELOG/unreleased/current.md (%d bytes)\n", len(res.Current))
	keys := make([]string, 0, len(res.Buckets))
	for k := range res.Buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  bucket: CHANGELOG/%s (%d bytes)\n", k, len(res.Buckets[k]))
	}
	relKeys := make([]string, 0, len(res.Released))
	for k := range res.Released {
		relKeys = append(relKeys, k)
	}
	sort.Strings(relKeys)
	for _, k := range relKeys {
		fmt.Printf("  release: CHANGELOG/%s (%d bytes)\n", k, len(res.Released[k]))
	}
}

func trimTrailingBlankLines(in []string) []string {
	out := append([]string(nil), in...)
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}
