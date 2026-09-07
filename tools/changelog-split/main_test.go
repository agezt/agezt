// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeResult is the test-only convenience wrapper around writeResultForce with
// both opt-ins off, i.e. the non-destructive default every emit starts from.
//
// It lives in a _test.go file deliberately. Production has exactly one emit call
// site (main), and that one passes the real --force / --discard-working-set
// flags, so a wrapper hard-coding false/false has no caller outside these tests.
// While it sat in main.go, `go run ./tools/deadcodecheck` failed with
// "unreachable func: writeResult"; the checker's own guidance is to move
// same-package test-only helpers into a _test.go rather than widen its
// cross-package allowlist, which is what three other symbols did on 2026-08-12.
// Do not move this back into main.go.
func writeResult(mainPath, outDir string, res splitResult) error {
	return writeResultForce(mainPath, outDir, res, false, false)
}

const sample = `# Changelog

Intro line.

## [Unreleased]

### Added
- **M101** one thing.
- **M145** another thing.

### Fixed
- no milestone here.

### Changed
- **M923** latest tracked phase.

### Security
- **M1002** changelog-only later phase.

## [1.0.0] — 2026-06-03

### Added
- released thing

## [0.1.0] — 2026-05-30

### Added
- first release
`

// vSample mirrors the header form the real root CHANGELOG.md actually uses
// (`## [v1.1.0] — 2026-09-01`), as opposed to `sample`'s unprefixed
// `## [1.0.0]`. The capture group keeps the leading `v`, so `vFilename` must not
// blindly prefix another one.
const vSample = `# Changelog

Intro line.

## [Unreleased]

### Fixed
- no milestone here.

## [v1.1.0] — 2026-09-01

### Added
- released thing
`

func TestVFilename(t *testing.T) {
	cases := []struct {
		name string
		tag  string
		want string
	}{
		{"unprefixed tag gains the v", "1.0.0", "v1.0.0.md"},
		{"already-prefixed tag is not doubled", "v1.1.0", "v1.1.0.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := vFilename(versionBlock{Tag: tc.tag}); got != tc.want {
				t.Fatalf("vFilename(%q) = %q, want %q", tc.tag, got, tc.want)
			}
		})
	}
}

// TestBuildSplitVPrefixedTag is the real-tree fingerprint: a `## [vX.Y.Z]` header
// must land in Released under `vX.Y.Z.md`. Pre-fix it keyed as `vvX.Y.Z.md`, so
// `--verify` reported `missing CHANGELOG\vv1.1.0.md` and a `--emit` would have
// written that stray file while removeStaleSplitFiles pruned the real one.
func TestBuildSplitVPrefixedTag(t *testing.T) {
	res, err := buildSplit(vSample)
	if err != nil {
		t.Fatalf("buildSplit: %v", err)
	}
	got, ok := res.Released["v1.1.0.md"]
	if !ok {
		keys := make([]string, 0, len(res.Released))
		for k := range res.Released {
			keys = append(keys, k)
		}
		t.Fatalf("Released keyed %v, want v1.1.0.md", keys)
	}
	if !strings.Contains(got, "## [v1.1.0] — 2026-09-01") {
		t.Fatalf("released v1.1.0 missing header, got %q", got)
	}
	if _, bad := res.Released["vv1.1.0.md"]; bad {
		t.Fatalf("Released must not contain the doubled-v key vv1.1.0.md")
	}
	if !strings.Contains(res.MainChangelog, "`v1.1.0.md` — `v1.1.0` (2026-09-01)") {
		t.Fatalf("release index bullet has a doubled-v filename: %s", res.MainChangelog)
	}
}

func TestBucketFor(t *testing.T) {
	cases := []struct {
		name   string
		header string
		body   []string
		want   string
	}{
		{"current when no M", "### Fixed", []string{"- no milestone"}, "current"},
		{"100 range", "### Added", []string{"- M145 added"}, "m100-m199"},
		{"600 first half", "### Added", []string{"- M623 added"}, "m600-m649"},
		{"600 second half", "### Added", []string{"- M688 added"}, "m650-m699"},
		{"900 range", "### Changed", []string{"- M923 changed"}, "m900-m999"},
		{"1000 plus", "### Security", []string{"- M1002 secure"}, "m1000+"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bucketFor(tc.header, tc.body); got != tc.want {
				t.Fatalf("bucketFor() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildSplit(t *testing.T) {
	res, err := buildSplit(sample)
	if err != nil {
		t.Fatalf("buildSplit: %v", err)
	}
	if !strings.Contains(res.MainChangelog, "See `CHANGELOG/unreleased/current.md`") {
		t.Fatalf("main changelog missing unreleased pointer")
	}
	if !strings.Contains(res.MainChangelog, "## Releases") || !strings.Contains(res.MainChangelog, "`v1.0.0.md` — `1.0.0` (2026-06-03)") {
		t.Fatalf("main changelog missing compact release index")
	}
	// Contract INVERTED on purpose. This asserted the opposite — that root must
	// not inline released bodies — but buildSplit requires a version block and
	// reads the working set out of root, so a root stripped of its blocks could
	// not be re-read and a second --emit died. Root now round-trips released
	// bodies so the tool is idempotent; the tree stays a derived copy.
	if !strings.Contains(res.MainChangelog, "### Added\n- released thing") {
		t.Fatalf("main changelog must keep released bodies readable for a re-emit")
	}
	if !strings.Contains(res.Current, "### Fixed") {
		t.Fatalf("current.md should keep unreleased chunk with no M refs")
	}
	if got := res.Buckets[filepath.ToSlash(filepath.Join("unreleased", "m100-m199.md"))]; !strings.Contains(got, "M101") || !strings.Contains(got, "M145") {
		t.Fatalf("m100-m199 bucket missing M101/M145 chunk")
	}
	if got := res.Buckets[filepath.ToSlash(filepath.Join("unreleased", "m900-m999.md"))]; !strings.Contains(got, "M923") {
		t.Fatalf("m900-m999 bucket missing M923 chunk")
	}
	if got := res.Buckets[filepath.ToSlash(filepath.Join("unreleased", "m1000+.md"))]; !strings.Contains(got, "M1002") {
		t.Fatalf("m1000+ bucket missing M1002 chunk")
	}
	if got := res.Released["v1.0.0.md"]; !strings.Contains(got, "## [1.0.0] — 2026-06-03") {
		t.Fatalf("released v1.0.0 missing header")
	}
}

func TestWriteAndVerify(t *testing.T) {
	res, err := buildSplit(sample)
	if err != nil {
		t.Fatalf("buildSplit: %v", err)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "CHANGELOG")
	mainPath := filepath.Join(dir, "CHANGELOG.md")
	if err := writeResult(mainPath, out, res); err != nil {
		t.Fatalf("writeResult: %v", err)
	}
	for _, path := range []string{
		mainPath,
		filepath.Join(out, "README.md"),
		filepath.Join(out, "REORG-LOG.md"),
		filepath.Join(out, "unreleased", "current.md"),
		filepath.Join(out, "unreleased", "m100-m199.md"),
		filepath.Join(out, "unreleased", "m900-m999.md"),
		filepath.Join(out, "unreleased", "m1000+.md"),
		filepath.Join(out, "v1.0.0.md"),
		filepath.Join(out, "v0.1.0.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected file %s: %v", path, err)
		}
	}
	if err := verifyResult(out, res); err != nil {
		t.Fatalf("verifyResult should pass after emit: %v", err)
	}
	// Stale output the tool DID generate should be pruned on a later emit. The
	// planted file carries the generated sentinel for exactly that reason: an
	// unmarked file is hand-held content the tool must never delete (root
	// CHANGELOG.md cannot regenerate it), which is the tree-is-canonical rule.
	stale := filepath.Join(out, "unreleased", "m600-m699.md")
	if err := os.WriteFile(stale, []byte("# Changelog — m600-m699\n\nold\n\n"+generatedSentinel+"\n"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	if err := writeResult(mainPath, out, res); err != nil {
		t.Fatalf("rewrite after stale file: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale split file should be removed, stat err=%v", err)
	}
	// mutate one file to prove verify fails.
	if err := os.WriteFile(filepath.Join(out, "README.md"), []byte("bad"), 0o644); err != nil {
		t.Fatalf("mutate README: %v", err)
	}
	if err := verifyResult(out, res); err == nil {
		t.Fatalf("verifyResult should fail on drift")
	}
}
