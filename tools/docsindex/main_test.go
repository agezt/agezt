// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDoc writes a document into the tree.
func writeDoc(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// archivedDoc is a document that carries the archive banner.
const archivedDoc = "# A stale plan\n\n> **Status:** ARCHIVED — superseded by the W program.\n\nBody.\n"

// liveDoc is a document without the banner.
const liveDoc = "# A live guide\n\nBody.\n"

// TestTheGateAcceptsAnIndexThatAgrees: the happy path — a live document
// listed under a live section and an archived one listed under the archive
// section.
func TestTheGateAcceptsAnIndexThatAgrees(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "LIVE.md", liveDoc)
	writeDoc(t, dir, "STALE.md", archivedDoc)
	index := "# Index\n\n" +
		"## Live documents\n\n" +
		"| [LIVE.md](LIVE.md) | the live one |\n\n" +
		"## Archived documents\n\n" +
		"| [STALE.md](STALE.md) | the stale one |\n"
	writeDoc(t, dir, "index.md", index)

	if errs := Check(dir, filepath.Join(dir, "index.md")); len(errs) != 0 {
		t.Fatalf("the gate rejected a good index: %v", errs)
	}
}

// TestTheGateCatchesALinkToNothing: a row that points at a file which is not
// there is the failure a reader hits first.
func TestTheGateCatchesALinkToNothing(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "LIVE.md", liveDoc)
	index := "# Index\n\n## Live documents\n\n| [GONE.md](GONE.md) | a doc that was renamed away |\n"
	writeDoc(t, dir, "index.md", index)

	errs := Check(dir, filepath.Join(dir, "index.md"))
	if !hasErrorContaining(errs, "GONE.md") {
		t.Fatalf("the gate missed the dead link: %v", errs)
	}
}

// TestTheGateCatchesADocumentNobodyLists: a document in the tree that the
// index never mentions is invisible — it is still there, and nobody finds it.
func TestTheGateCatchesADocumentNobodyLists(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "LIVE.md", liveDoc)
	writeDoc(t, dir, "HIDDEN.md", liveDoc)
	index := "# Index\n\n## Live documents\n\n| [LIVE.md](LIVE.md) | the live one |\n"
	writeDoc(t, dir, "index.md", index)

	errs := Check(dir, filepath.Join(dir, "index.md"))
	if !hasErrorContaining(errs, "HIDDEN.md") {
		t.Fatalf("the gate missed the unlisted document: %v", errs)
	}
}

// TestTheGateCatchesAnArchivedDocumentListedAsLive: the banner alone does not
// archive anything. This is the state four documents sat in — bannered and
// still listed as current.
func TestTheGateCatchesAnArchivedDocumentListedAsLive(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "STALE.md", archivedDoc)
	index := "# Index\n\n## Live documents\n\n| [STALE.md](STALE.md) | still sold as live |\n"
	writeDoc(t, dir, "index.md", index)

	errs := Check(dir, filepath.Join(dir, "index.md"))
	if !hasErrorContaining(errs, "ARCHIVED banner") {
		t.Fatalf("the gate missed the archived document listed as live: %v", errs)
	}
}

// TestTheGateCatchesALiveDocumentListedAsArchived: the other half — the
// archive section must not hold a document that was never archived.
func TestTheGateCatchesALiveDocumentListedAsArchived(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "LIVE.md", liveDoc)
	index := "# Index\n\n## Archived documents\n\n| [LIVE.md](LIVE.md) | wrongly archived |\n"
	writeDoc(t, dir, "index.md", index)

	errs := Check(dir, filepath.Join(dir, "index.md"))
	if !hasErrorContaining(errs, "no ARCHIVED banner") {
		t.Fatalf("the gate missed the live document listed as archived: %v", errs)
	}
}

// TestTheGateIgnoresTheIndexItself: the index does not list itself, and
// requiring it to would be a rule about the shape of the file rather than
// about the tree.
func TestTheGateIgnoresTheIndexItself(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "LIVE.md", liveDoc)
	index := "# Index\n\n## Live documents\n\n| [LIVE.md](LIVE.md) | the live one |\n"
	writeDoc(t, dir, "index.md", index)

	if errs := Check(dir, filepath.Join(dir, "index.md")); len(errs) != 0 {
		t.Fatalf("the gate rejected the index for not listing itself: %v", errs)
	}
}

// TestTheRepositoriesOwnIndexAgreesWithTheDocsTree: the gate run against the
// repository itself.
//
// The unit tests above prove the gate can fail; this one proves the tree is
// in the state the gate asks for.
func TestTheRepositoriesOwnIndexAgreesWithTheDocsTree(t *testing.T) {
	dir := filepath.Join("..", "..", "docs")
	index := filepath.Join(dir, "index.md")
	if _, err := os.Stat(index); err != nil {
		t.Fatalf("the repository has no documentation index: %v", err)
	}
	if errs := Check(dir, index); len(errs) != 0 {
		for _, e := range errs {
			t.Errorf("the repository's own index: %v", e)
		}
	}
}

// hasErrorContaining reports whether one of the errors mentions the needle.
func hasErrorContaining(errs []error, needle string) bool {
	for _, e := range errs {
		if strings.Contains(e.Error(), needle) {
			return true
		}
	}
	return false
}
