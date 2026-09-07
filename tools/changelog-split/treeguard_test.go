// SPDX-License-Identifier: MIT

package main

// Durable regressions for the tree loss gate.
//
// The gate used to hardcode one path (unreleased/current.md), so `--emit --force`
// could replace a hand-held milestone bucket — the only copy of its content — and
// still exit 0. Two invariants keep that from coming back:
//
//  1. every tree file the run writes is guarded, not just the working set;
//  2. the guard's line index must cover EVERY generated document, because a
//     generated file missing from its own index reads as fully orphaned and
//     refuses an ordinary re-emit (this is exactly how the first version of the
//     generalized gate broke emit idempotency).

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestForceCannotDiscardAHandHeldBucket drives the real production path: a tree
// the tool owns, then one bucket replaced by hand-held content that appears in
// no generated output. The forced emit must refuse and leave the bucket intact.
func TestForceCannotDiscardAHandHeldBucket(t *testing.T) {
	res, err := buildSplit(sample)
	if err != nil {
		t.Fatalf("buildSplit: %v", err)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "CHANGELOG")
	mainPath := filepath.Join(dir, "CHANGELOG.md")

	// Round one: nothing exists yet, so every target is written and owned.
	if err := writeResult(mainPath, out, res); err != nil {
		t.Fatalf("first emit: %v", err)
	}

	keys := make([]string, 0, len(res.Buckets))
	for k := range res.Buckets {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		t.Fatal("fixture produces no buckets; the sample must reference milestones")
	}
	sort.Strings(keys)
	bucketRel := keys[0]
	bucket := filepath.Join(out, filepath.FromSlash(bucketRel))

	const soleCopy = "- SOLE-COPY bucket history that exists in no other file"
	if err := os.WriteFile(bucket, []byte("# Changelog — hand-held\n\n"+soleCopy+"\n"), 0o644); err != nil {
		t.Fatalf("plant bucket: %v", err)
	}

	// --force adopts ownership, but adoption is not a licence to discard.
	err = writeResultForce(mainPath, out, res, true, false)
	if err == nil {
		t.Fatalf("--emit --force destroyed the hand-held bucket %s and reported success", bucketRel)
	}
	if got := mustRead(t, bucket); !strings.Contains(got, soleCopy) {
		t.Fatalf("a refused emit must leave the tree untouched; bucket now holds:\n%s", got)
	}
}

// TestReEmitOverOwnedTreeIsNotRefused pins the completeness of the line index.
// Once the tool owns every tree file, re-emitting the same result is a no-op and
// must never trip the loss gate. If generatedLineIndex ever stops indexing a
// document it generates, this fails on that file instead of silently destroying
// it — the failure mode that first appeared as a refused README.md.
func TestReEmitOverOwnedTreeIsNotRefused(t *testing.T) {
	res, err := buildSplit(sample)
	if err != nil {
		t.Fatalf("buildSplit: %v", err)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "CHANGELOG")
	mainPath := filepath.Join(dir, "CHANGELOG.md")
	if err := writeResult(mainPath, out, res); err != nil {
		t.Fatalf("first emit: %v", err)
	}

	kept := generatedLineIndex(res)
	targets := []string{
		filepath.Join(out, "README.md"),
		filepath.Join(out, "REORG-LOG.md"),
		filepath.Join(out, "unreleased", "current.md"),
	}
	for _, k := range sortedKeys(res) {
		targets = append(targets, filepath.Join(out, filepath.FromSlash(k)))
	}
	for _, path := range targets {
		lost, err := lostLines(path, kept)
		if err != nil {
			t.Fatalf("lostLines(%s): %v", filepath.ToSlash(path), err)
		}
		if len(lost) > 0 {
			t.Errorf("a file the tool just generated reports %d lost line(s); generatedLineIndex is missing a document. first lost line: %q",
				len(lost), lost[0])
		}
	}

	if err := writeResultForce(mainPath, out, res, true, false); err != nil {
		t.Fatalf("re-emit over an owned tree must not be refused: %v", err)
	}
}

// sortedKeys returns bucket and released-document keys, sorted for determinism.
func sortedKeys(res splitResult) []string {
	keys := make([]string, 0, len(res.Buckets)+len(res.Released))
	for k := range res.Buckets {
		keys = append(keys, k)
	}
	for k := range res.Released {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
