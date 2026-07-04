// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestBucketFor(t *testing.T) {
	cases := []struct {
		name string
		header string
		body []string
		want string
	}{
		{"current when no M", "### Fixed", []string{"- no milestone"}, "current"},
		{"100 range", "### Added", []string{"- M145 added"}, "m100-m199"},
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
	// mutate one file to prove verify fails.
	if err := os.WriteFile(filepath.Join(out, "README.md"), []byte("bad"), 0o644); err != nil {
		t.Fatalf("mutate README: %v", err)
	}
	if err := verifyResult(out, res); err == nil {
		t.Fatalf("verifyResult should fail on drift")
	}
}
