// SPDX-License-Identifier: MIT

package artifact_test

import (
	"testing"

	"github.com/agezt/agezt/kernel/artifact"
)

func openIdx(t *testing.T) (*artifact.Index, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := artifact.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	idx, err := artifact.OpenIndex(st, dir)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	return idx, dir
}

func TestIndex_PutListGetBytes(t *testing.T) {
	idx, _ := openIdx(t)

	e1, err := idx.PutEntry(artifact.Entry{Kind: "image", Source: "telegram", Mime: "image/png", Caption: "a cat"}, []byte("PNGDATA"), 100)
	if err != nil {
		t.Fatalf("PutEntry: %v", err)
	}
	if e1.ID == "" || e1.Ref == "" || e1.Size != 7 || e1.CreatedMs != 100 {
		t.Fatalf("entry not filled in: %+v", e1)
	}
	_, err = idx.PutEntry(artifact.Entry{Kind: "tool-output", Source: "run"}, []byte("log output"), 200)
	if err != nil {
		t.Fatalf("PutEntry 2: %v", err)
	}

	// List newest-first.
	all := idx.List(artifact.Filter{})
	if len(all) != 2 || all[0].CreatedMs != 200 {
		t.Fatalf("List = %d entries, newest-first broken: %+v", len(all), all)
	}
	// Filter by kind.
	imgs := idx.List(artifact.Filter{Kind: "image"})
	if len(imgs) != 1 || imgs[0].ID != e1.ID {
		t.Fatalf("kind filter = %+v", imgs)
	}

	// Bytes round-trips through the blob store.
	b, e, err := idx.Bytes(e1.ID)
	if err != nil || string(b) != "PNGDATA" || e.Caption != "a cat" {
		t.Fatalf("Bytes = %q,%+v,%v", b, e, err)
	}
}

func TestIndex_DeleteGCsOrphanBlobButKeepsShared(t *testing.T) {
	idx, _ := openIdx(t)

	// Two arrivals of the SAME bytes → two entries, one blob (content dedup).
	a, _ := idx.PutEntry(artifact.Entry{Kind: "image", Source: "telegram"}, []byte("SAME"), 1)
	bEnt, _ := idx.PutEntry(artifact.Entry{Kind: "image", Source: "slack"}, []byte("SAME"), 2)
	if a.Ref != bEnt.Ref {
		t.Fatalf("identical bytes should share a ref: %s vs %s", a.Ref, bEnt.Ref)
	}

	// Delete one — the blob must survive because the other entry still uses it.
	if err := idx.Delete(a.ID); err != nil {
		t.Fatalf("Delete a: %v", err)
	}
	if _, _, err := idx.Bytes(bEnt.ID); err != nil {
		t.Fatalf("shared blob GC'd too early: %v", err)
	}
	if idx.Count() != 1 {
		t.Fatalf("Count after one delete = %d, want 1", idx.Count())
	}

	// Delete the last referrer — now the blob is orphaned and GC'd.
	if err := idx.Delete(bEnt.ID); err != nil {
		t.Fatalf("Delete b: %v", err)
	}
	if idx.Count() != 0 {
		t.Fatalf("Count = %d, want 0", idx.Count())
	}
}

func TestIndex_IndexRefForAlreadyStoredBlob(t *testing.T) {
	dir := t.TempDir()
	st, _ := artifact.Open(dir)
	idx, _ := artifact.OpenIndex(st, dir)

	// Simulate the agent's offload: bytes are already in the blob store by ref.
	ref, err := st.Put([]byte("a big tool output …"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	e, err := idx.IndexRef(ref, artifact.Entry{Kind: "tool-output", Source: "run", Name: "shell-output.txt", Corr: "run-1"}, 42)
	if err != nil {
		t.Fatalf("IndexRef: %v", err)
	}
	if e.Ref != ref || e.Size == 0 || e.CreatedMs != 42 || e.Corr != "run-1" {
		t.Fatalf("entry not filled from store: %+v", e)
	}
	got := idx.List(artifact.Filter{Kind: "tool-output"})
	if len(got) != 1 || got[0].ID != e.ID {
		t.Fatalf("tool-output not listed: %+v", got)
	}

	// A ref that isn't in the store is rejected (no phantom entry).
	if _, err := idx.IndexRef("00ff", artifact.Entry{Kind: "tool-output"}, 1); err == nil {
		t.Error("IndexRef of an absent ref should fail")
	}
}

func TestIndex_PersistsAcrossReopen(t *testing.T) {
	idx, dir := openIdx(t)
	e, _ := idx.PutEntry(artifact.Entry{Kind: "image", Source: "telegram", Caption: "kept"}, []byte("X"), 5)

	// Reopen against the same dir — the entry must reload from its sidecar file.
	st, _ := artifact.Open(dir)
	idx2, err := artifact.OpenIndex(st, dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, ok := idx2.Get(e.ID)
	if !ok || got.Caption != "kept" {
		t.Fatalf("entry not persisted across reopen: %+v ok=%v", got, ok)
	}
}

func TestIndex_StaleAndCollect(t *testing.T) {
	idx, _ := openIdx(t)
	// created_ms: old=100, mid=500, new=1000.
	old, _ := idx.PutEntry(artifact.Entry{Kind: "tool-output", Source: "run"}, []byte("oldlog"), 100)
	_, _ = idx.PutEntry(artifact.Entry{Kind: "tool-output", Source: "run"}, []byte("midlog"), 500)
	newE, _ := idx.PutEntry(artifact.Entry{Kind: "image", Source: "telegram"}, []byte("freshpng"), 1000)

	// Stale before 600 → the two oldest (100, 500), oldest-first.
	stale := idx.StaleEntries(600)
	if len(stale) != 2 || stale[0].ID != old.ID {
		t.Fatalf("StaleEntries(600) = %d entries, first=%v want 2 oldest-first", len(stale), stale[0].ID)
	}
	// An entry with no created time is never stale.
	noTime, _ := idx.PutEntry(artifact.Entry{Kind: "x"}, []byte("notime"), 0)
	for _, e := range idx.StaleEntries(1 << 62) {
		if e.ID == noTime.ID {
			t.Error("an entry with created_ms<=0 must not be considered stale")
		}
	}

	// Collect before 600 removes the two oldest and reports their byte sum.
	n, bytes := idx.Collect(600)
	if n != 2 || bytes != int64(len("oldlog")+len("midlog")) {
		t.Fatalf("Collect(600) = (%d, %d), want (2, %d)", n, bytes, len("oldlog")+len("midlog"))
	}
	if _, ok := idx.Get(old.ID); ok {
		t.Error("old entry should be gone after Collect")
	}
	if _, ok := idx.Get(newE.ID); !ok {
		t.Error("fresh entry must survive Collect")
	}
}
