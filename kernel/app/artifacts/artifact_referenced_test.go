// SPDX-License-Identifier: MIT

package artifacts_test

import (
	"strings"
	"testing"

	apparifacts "github.com/agezt/agezt/kernel/app/artifacts"
	blobs "github.com/agezt/agezt/kernel/modules/artifacts/artifact"
)

// TestArtifactDeleteReportsAJournalReferencedArtifact: the index refuses to
// remove a blob the journal still names (a tool.result's raw_ref). The
// operation must say so in words the operator can act on — the raw sentinel
// would leak the module's internals into the CLI and the console.
func TestArtifactDeleteReportsAJournalReferencedArtifact(t *testing.T) {
	idx := &index{cause: blobs.ErrStillReferenced}
	service := apparifacts.New(nil, idx, nil)
	_, err := service.Delete(t.Context(), apparifacts.DeleteInput{ID: "art-1"})
	if err == nil {
		t.Fatal("the delete of a journal-referenced artifact was allowed")
	}
	if !strings.Contains(err.Error(), "still referenced by the journal") {
		t.Errorf("error = %q, want it to name the journal reference", err)
	}
	if !strings.Contains(err.Error(), "art-1") {
		t.Errorf("error = %q, want it to name the artifact", err)
	}
}

// TestArtifactDeleteKeepsTheNotFoundWording: the new case must not swallow
// the old one.
func TestArtifactDeleteKeepsTheNotFoundWording(t *testing.T) {
	idx := &index{cause: blobs.ErrNotFound}
	service := apparifacts.New(nil, idx, nil)
	_, err := service.Delete(t.Context(), apparifacts.DeleteInput{ID: "art-1"})
	if err == nil || !strings.Contains(err.Error(), "artifact not found") {
		t.Fatalf("error = %v, want the not-found wording", err)
	}
}
