// SPDX-License-Identifier: MIT

package main

// Non-idempotency regression. `renderMain` used to replace the root changelog's
// released version blocks (and its Unreleased content) with a pointer plus a
// `## Releases` index. But `buildSplit` REQUIRES at least one version block and
// reads the working set out of root, so the file the tool wrote back could not
// be read again: a second `--emit` against the same root — or `--verify` after
// `--emit` — died with "no released version blocks found".
//
// Root is where a human authors notes, so it must round-trip: keep released
// blocks readable AND keep the Unreleased body, or the second emit regenerates
// current.md from a stub and the working-set loss gate (rightly) refuses it.
//
// Also pins the parser rule that the fix depends on: a version body must end at
// the next `## ` header of ANY kind. With the old "next version header, else
// EOF" rule, the regenerated `## Releases` index trailing the last block would
// be swallowed into that release's body and copied into its tree file.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readStr(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func writeStr(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestEmitTwiceAgainstSameRootIsIdempotent is the headline: two emits driven by
// one root file, with no error and no drift, and the root stops changing.
func TestEmitTwiceAgainstSameRootIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "CHANGELOG.md")
	out := filepath.Join(dir, "CHANGELOG")
	writeStr(t, src, sample)

	res1, err := buildSplit(readStr(t, src))
	if err != nil {
		t.Fatalf("buildSplit(emit #1): %v", err)
	}
	if err := writeResult(src, out, res1); err != nil {
		t.Fatalf("emit #1: %v", err)
	}
	root1 := readStr(t, src)

	// Pre-fix this is where the tool dies: it cannot re-read its own output.
	res2, err := buildSplit(root1)
	if err != nil {
		t.Fatalf("--emit wrote a root the tool cannot re-read (non-idempotent): %v\n--- emitted root ---\n%s", err, root1)
	}
	if err := writeResult(src, out, res2); err != nil {
		t.Fatalf("emit #2 failed although it reads the same root: %v", err)
	}
	root2 := readStr(t, src)
	if root1 != root2 {
		t.Errorf("root changed between emit #1 and emit #2 — not a fixed point\nemit1 %d bytes, emit2 %d bytes\n--- emit2 root ---\n%s", len(root1), len(root2), root2)
	}

	// Released blocks stay readable in root and still regenerate their file.
	for _, want := range []string{"## [1.0.0] — 2026-06-03", "- released thing", "## [0.1.0] — 2026-05-30", "- first release"} {
		if !strings.Contains(root2, want) {
			t.Errorf("emitted root lost released content %q", want)
		}
	}
	if got := res2.Released["v1.0.0.md"]; !strings.Contains(got, "- released thing") {
		t.Errorf("regenerated v1.0.0.md lost its body: %q", got)
	}

	// The working set still round-trips through root, so emit #2 does not try to
	// downgrade current.md to a stub.
	if !strings.Contains(root2, "- no milestone here.") {
		t.Error("emitted root dropped the Unreleased working-set content")
	}
	if got := mustRead(t, filepath.Join(out, "unreleased", "current.md")); !strings.Contains(got, "- no milestone here.") {
		t.Errorf("emit #2 truncated the working set: %q", got)
	}

	// The index must not leak into the last release's body.
	if got := res2.Released["v0.1.0.md"]; strings.Contains(got, "## Releases") {
		t.Errorf("`## Releases` index swallowed into v0.1.0.md body: %q", got)
	}
	// The compact index and the pointer survive, and verify can check the tree.
	for _, want := range []string{"## Releases", "`v1.0.0.md` — `1.0.0` (2026-06-03)", "See `CHANGELOG/unreleased/current.md`"} {
		if !strings.Contains(root2, want) {
			t.Errorf("emitted root missing %q", want)
		}
	}
	if err := verifyResult(out, res2); err != nil {
		t.Fatalf("--verify after two emits should be clean: %v", err)
	}
}

// TestBuildSplitStopsVersionBodyAtAnySectionHeader pins the parser rule itself.
func TestBuildSplitStopsVersionBodyAtAnySectionHeader(t *testing.T) {
	src := "# Changelog\n\n## [Unreleased]\n\n### Added\n\n- **M101** work\n\n" +
		"## [1.0.0] — 2026-06-03\n\n### Added\n\n- released thing\n\n## Contributors\n\n- nobody\n"
	res, err := buildSplit(src)
	if err != nil {
		t.Fatalf("buildSplit: %v", err)
	}
	body := res.Released["v1.0.0.md"]
	if strings.Contains(body, "## Contributors") || strings.Contains(body, "nobody") {
		t.Errorf("a non-version `## ` section leaked into the release body: %q", body)
	}
	if !strings.Contains(body, "- released thing") {
		t.Errorf("release body lost its own content: %q", body)
	}
}
