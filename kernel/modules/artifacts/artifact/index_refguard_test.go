// SPDX-License-Identifier: MIT

package artifact

import (
	"errors"
	"testing"
)

// refGuard is a fake RefGuard: it protects the refs it is given, and records
// what it was asked about (the module must ask once per operation, not once
// per candidate).
type refGuard struct {
	protected map[string]bool
	asked     [][]string
}

func (g *refGuard) ReferencedRefs(candidates []string) map[string]bool {
	g.asked = append(g.asked, append([]string(nil), candidates...))
	out := map[string]bool{}
	for _, c := range candidates {
		if g.protected[c] {
			out[c] = true
		}
	}
	return out
}

func openIndexed(t *testing.T) (*Store, *Index) {
	t.Helper()
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	idx, err := OpenIndex(store, dir)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	return store, idx
}

// TestCollect_KeepsTheBlobTheJournalStillNames is the proof of the bug.
//
// A tool output that is too large to inline is offloaded into the blob store
// and the journaled tool.result carries the blob's raw_ref instead of the
// bytes. The journal is append-only with full retention, so that name is
// forever. The collector used to remove the blob anyway — the raw_ref then
// pointed at nothing, `agt artifact_get <ref>` failed, and the run's output was
// gone while the journal still claimed the bytes were there.
func TestCollect_KeepsTheBlobTheJournalStillNames(t *testing.T) {
	store, idx := openIndexed(t)
	data := []byte("a very large tool output")
	ref, err := store.Put(data)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	// The indexer records the blob the journal named (IndexRef).
	if _, err := idx.IndexRef(ref, Entry{Kind: "tool-output", Source: "run", Name: "out.txt"}, 1); err != nil {
		t.Fatalf("IndexRef: %v", err)
	}
	guard := &refGuard{protected: map[string]bool{ref: true}}
	idx.SetRefGuard(guard)

	// The entry is old enough to be a collection candidate.
	candidates := idx.StaleEntries(1000)
	if len(candidates) != 0 {
		t.Fatalf("a journal-named blob is a collection candidate: %+v", candidates)
	}

	collected, _ := idx.Collect(1000)
	if collected != 0 {
		t.Errorf("collected %d entries, want 0 (the journal still names the blob)", collected)
	}
	has, err := store.Has(ref)
	if err != nil {
		t.Fatalf("has: %v", err)
	}
	if !has {
		t.Error("the blob the journal still names was garbage-collected — the raw_ref now points at nothing")
	}
}

// TestDelete_RefusesABlobTheJournalStillNames: the explicit delete is refused
// with the named error rather than silently breaking the journal's reference.
func TestDelete_RefusesABlobTheJournalStillNames(t *testing.T) {
	store, idx := openIndexed(t)
	ref, err := store.Put([]byte("bytes"))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	entry, err := idx.IndexRef(ref, Entry{Kind: "tool-output", Source: "run"}, 1)
	if err != nil {
		t.Fatalf("IndexRef: %v", err)
	}
	idx.SetRefGuard(&refGuard{protected: map[string]bool{ref: true}})

	err = idx.Delete(entry.ID)
	if !errors.Is(err, ErrStillReferenced) {
		t.Fatalf("Delete error = %v, want ErrStillReferenced", err)
	}
	if _, ok := idx.Get(entry.ID); !ok {
		t.Error("the entry was forgotten even though the delete was refused")
	}
	if has, _ := store.Has(ref); !has {
		t.Error("the blob was removed even though the delete was refused")
	}
}

// TestCollect_StillReapsTheBlobsTheJournalDoesNotName: the protection is
// narrow. An artifact the journal never named (an inbound image, say) is
// collected exactly as before.
func TestCollect_StillReapsTheBlobsTheJournalDoesNotName(t *testing.T) {
	store, idx := openIndexed(t)
	ref, err := store.Put([]byte("standalone bytes"))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	entry, err := idx.PutEntry(Entry{Kind: "image", Source: "inbound"}, []byte("standalone bytes"), 1)
	if err != nil {
		t.Fatalf("PutEntry: %v", err)
	}
	idx.SetRefGuard(&refGuard{protected: map[string]bool{"other-ref": true}})

	if got := len(idx.StaleEntries(1000)); got != 1 {
		t.Fatalf("stale candidates = %d, want 1", got)
	}
	collected, bytes := idx.Collect(1000)
	if collected != 1 || bytes != int64(len("standalone bytes")) {
		t.Errorf("Collect = (%d, %d), want (1, %d)", collected, bytes, len("standalone bytes"))
	}
	if has, _ := store.Has(ref); has {
		t.Error("the standalone blob survived the collector")
	}
	if _, ok := idx.Get(entry.ID); ok {
		t.Error("the standalone entry survived the collector")
	}
}

// TestCollect_AsksTheGuardOncePerOperation: the guard is asked with every
// candidate at once — a host that answers from one pass over its event log
// must not be asked once per candidate.
func TestCollect_AsksTheGuardOncePerOperation(t *testing.T) {
	_, idx := openIndexed(t)
	for i := 0; i < 3; i++ {
		if _, err := idx.PutEntry(Entry{Kind: "image"}, []byte{byte(i)}, 1); err != nil {
			t.Fatalf("PutEntry: %v", err)
		}
	}
	guard := &refGuard{}
	idx.SetRefGuard(guard)
	idx.Collect(1000)
	if len(guard.asked) != 1 {
		t.Fatalf("the guard was asked %d times, want 1 (once per operation)", len(guard.asked))
	}
	if len(guard.asked[0]) != 3 {
		t.Errorf("the guard was asked about %d candidates, want 3", len(guard.asked[0]))
	}
}
