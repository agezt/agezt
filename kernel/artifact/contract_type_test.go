// SPDX-License-Identifier: MIT

package artifact_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/contract/remoteapi"
)

// TestEntryIsTheContractRow: the store's row IS the contract row — the same
// declaration the REST adapter serves and the peer call reads — so the stored
// row and the served row cannot drift apart.
func TestEntryIsTheContractRow(t *testing.T) {
	if reflect.TypeOf(artifact.Entry{}) != reflect.TypeOf(remoteapi.Artifact{}) {
		t.Fatalf("the stored row drifted from the contract row: %v vs %v", reflect.TypeOf(artifact.Entry{}), reflect.TypeOf(remoteapi.Artifact{}))
	}
	var row remoteapi.Artifact = artifact.Entry{ID: "art-1", Ref: "ref-1", Size: 3, CreatedMS: 7}
	if row.ID != "art-1" || row.Ref != "ref-1" || row.Size != 3 || row.CreatedMS != 7 {
		t.Fatal(row)
	}
}

// TestIndexPersistsTheSameShape: the stored metadata file keeps the shape it
// always had — the same keys, in the same order, with the empty ones omitted.
func TestIndexPersistsTheSameShape(t *testing.T) {
	idx, dir := openIdx(t)
	entry, err := idx.PutEntry(artifact.Entry{Kind: "image", Source: "telegram", Mime: "image/png", Caption: "a cat"}, []byte("PNGDATA"), 100)
	if err != nil {
		t.Fatalf("PutEntry: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "index", entry.ID+".json"))
	if err != nil {
		t.Fatalf("read the stored metadata: %v", err)
	}
	got := string(raw)
	for _, key := range []string{`"id"`, `"ref"`, `"kind"`, `"source"`, `"mime"`, `"size"`, `"created_ms"`, `"caption"`} {
		if !strings.Contains(got, key) {
			t.Fatalf("the stored metadata lost %s: %s", key, got)
		}
	}
	// The empty ones stay out of the file, as they always were.
	for _, key := range []string{`"name"`, `"sender"`, `"corr"`} {
		if strings.Contains(got, key) {
			t.Fatalf("the stored metadata gained %s: %s", key, got)
		}
	}
}
