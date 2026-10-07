// SPDX-License-Identifier: MIT
package artifacts_test

import (
	"context"
	"errors"
	appartifacts "github.com/agezt/agezt/kernel/app/artifacts"
	blobs "github.com/agezt/agezt/kernel/artifact"
	"testing"
	"time"
)

func TestArtifactServicesUseActualIndexCutoffAndDedupOwnership(t *testing.T) {
	dir := t.TempDir()
	store, err := blobs.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := blobs.OpenIndex(store, dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(100 * 24 * 60 * 60 * 1000)
	cutoff := now.Add(-30 * 24 * time.Hour).UnixMilli()
	old, err := idx.PutEntry(blobs.Entry{Name: "old", Kind: "file", Source: "run", Corr: "owned"}, []byte("shared"), cutoff-1)
	if err != nil {
		t.Fatal(err)
	}
	keep, err := idx.PutEntry(blobs.Entry{Name: "boundary", Kind: "file", Source: "run", Corr: "owned"}, []byte("shared"), cutoff)
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := idx.PutEntry(blobs.Entry{Name: "no clock"}, []byte("unknown"), 0)
	if err != nil {
		t.Fatal(err)
	}
	service := appartifacts.New(store, idx, func() time.Time { return now })
	ctx := context.Background()
	dry, err := service.Collect(ctx, appartifacts.CollectInput{DryRun: true})
	if err != nil || dry.Count != 1 || dry.Candidates == nil || (*dry.Candidates)[0].ID != old.ID || idx.Count() != 3 {
		t.Fatal(dry, err, idx.Count())
	}
	collected, err := service.Collect(ctx, appartifacts.CollectInput{DryRun: false})
	if err != nil || collected.Count != 1 || collected.Bytes != int64(len("shared")) || collected.Candidates != nil || idx.Count() != 2 {
		t.Fatal(collected, err, idx.Count())
	}
	if _, err = store.Get(old.Ref); err != nil {
		t.Fatal("shared blob deleted with surviving reference", err)
	}
	if _, ok := idx.Get(keep.ID); !ok {
		t.Fatal("cutoff equality was collected")
	}
	if _, ok := idx.Get(unknown.ID); !ok {
		t.Fatal("unknown creation time was collected")
	}
	deleted, err := service.Delete(ctx, appartifacts.DeleteInput{ID: keep.ID})
	if err != nil || !deleted.Deleted {
		t.Fatal(deleted, err)
	}
	if _, err = store.Get(old.Ref); !errors.Is(err, blobs.ErrNotFound) {
		t.Fatal("final reference did not collect blob", err)
	}
}
