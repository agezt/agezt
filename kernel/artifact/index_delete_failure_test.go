// SPDX-License-Identifier: MIT
package artifact_test

import (
	"github.com/agezt/agezt/kernel/artifact"
	"os"
	"path/filepath"
	"testing"
)

func TestIndexDeleteDoesNotForgetEntryWhenMetadataRemovalFails(t *testing.T) {
	dir := t.TempDir()
	store, err := artifact.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := artifact.OpenIndex(store, dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := idx.PutEntry(artifact.Entry{Name: "owned"}, []byte("owned"), 100)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "index", entry.ID+".json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "blocker"), []byte("owned blocker"), 0600); err != nil {
		t.Fatal(err)
	}
	err = idx.Delete(entry.ID)
	_, found := idx.Get(entry.ID)
	_, blobErr := store.Get(entry.Ref)
	if err == nil || !found || blobErr != nil {
		t.Fatalf("EXPECTED: metadata removal failure returns cause and preserves entry/blob; ACTUAL: err=%v found=%v blobErr=%v", err, found, blobErr)
	}
}

func TestIndexCollectDoesNotCountOrForgetFailedMetadataDeletion(t *testing.T) {
	dir := t.TempDir()
	store, err := artifact.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := artifact.OpenIndex(store, dir)
	if err != nil {
		t.Fatal(err)
	}
	blocked, err := idx.PutEntry(artifact.Entry{Name: "blocked"}, []byte("blocked"), 100)
	if err != nil {
		t.Fatal(err)
	}
	good, err := idx.PutEntry(artifact.Entry{Name: "good"}, []byte("good"), 101)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "index", blocked.ID+".json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "blocker"), []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	count, bytes := idx.Collect(200)
	_, blockedFound := idx.Get(blocked.ID)
	_, goodFound := idx.Get(good.ID)
	_, blobErr := store.Get(blocked.Ref)
	if count != 1 || bytes != good.Size || !blockedFound || goodFound || blobErr != nil {
		t.Fatalf("EXPECTED: only successful deletion counted, failed entry/blob retained; ACTUAL: count=%d bytes=%d blocked=%v good=%v blobErr=%v", count, bytes, blockedFound, goodFound, blobErr)
	}
}
func TestIndexDeleteAllowsAlreadyAbsentMetadata(t *testing.T) {
	dir := t.TempDir()
	store, err := artifact.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := artifact.OpenIndex(store, dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := idx.PutEntry(artifact.Entry{Name: "owned"}, []byte("owned"), 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "index", entry.ID+".json")); err != nil {
		t.Fatal(err)
	}
	if err := idx.Delete(entry.ID); err != nil {
		t.Fatal("already absent metadata should stay idempotent", err)
	}
	if _, found := idx.Get(entry.ID); found {
		t.Fatal("entry retained after successful removal")
	}
}
