// SPDX-License-Identifier: MIT

package artifact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/agezt/agezt/internal/atomicfile"
	"github.com/agezt/agezt/kernel/contract/artifactapi"
	"github.com/agezt/agezt/kernel/contract/remoteapi"
	"github.com/agezt/agezt/kernel/ulid"
)

// Entry is one stored artifact's metadata — a single ARRIVAL. The bytes live in
// the content-addressed blob Store under Ref (deduped across arrivals); an Entry
// is per-arrival, so the same image sent twice is two Entries pointing at one
// blob. This is the queryable, deletable layer the file-manager (M823) and the
// inbound-image persistence (M822) build on, while Store stays a pure blob store.
//
// It is the contract row (kernel/contract/remoteapi) — the same declaration the
// REST adapter serves and the peer call reads — so the stored row and the
// served row cannot drift. The on-disk shape is unchanged.
type Entry = remoteapi.Artifact

// Filter narrows List; empty fields impose no constraint (exact match otherwise).
//
// It is the contract's — the query over the artifact row — because the tool
// that lists artifacts is a PLUGIN, and a plugin may reach the contract and
// the platform layers and nothing else. The name stays here as an alias.
type Filter = remoteapi.Filter

// RefGuard reports which blobs are still referenced from OUTSIDE the index —
// in practice by a journal event's raw_ref: the tool.result that offloaded a
// large output names the blob it wrote, and the journal keeps that name.
//
// The collector must never remove a blob the guard protects. The journal is
// the durable record of the run, so a raw_ref that points at deleted bytes is
// a dangling promise: `agt artifact_get <ref>` fails and the run's output is
// lost, while the journal still claims the bytes are there. The bytes belong
// to the journal until the journal forgets them.
//
// The guard is asked ONCE per operation, with the candidate refs, so the host
// can answer from a single pass over its event log instead of one pass per
// ref. A nil guard protects nothing (the collector then behaves as before).
//
// The PORT now lives in the contract layer (kernel/contract/artifactapi —
// W3.43): the host installs its guard on an index it does not own, so the
// guard has to be nameable from the contract. The name stays here as an
// alias.
type RefGuard = artifactapi.RefGuard

// The index answers the contract's ports: the host holds them, and the
// module owns the file. The tool-facing port and the host-facing one are
// separate on purpose — the module answers both.
var _ artifactapi.Index = (*Index)(nil)
var _ artifactapi.HostIndex = (*Index)(nil)

// Index is a metadata sidecar over a blob Store. Entries persist as one JSON file
// each under <baseDir>/index/<id>.json; the blob shards (<aa>/<ref>) are
// untouched. An in-memory map (loaded at Open) backs queries; all access is
// mutex-guarded. Deleting an Entry garbage-collects its blob only when no other
// Entry still references that ref — and never when the RefGuard says the
// journal still names those bytes.
type Index struct {
	store   *Store
	dir     string
	mu      sync.Mutex
	entries map[string]Entry
	guard   RefGuard
}

// OpenIndex opens (creating if needed) the metadata index for store, rooted at
// <baseDir>/index. baseDir is normally the same artifacts dir the Store uses.
func OpenIndex(store *Store, baseDir string) (*Index, error) {
	dir := filepath.Join(baseDir, "index")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("artifact index: open %s: %w", dir, err)
	}
	idx := &Index{store: store, dir: dir, entries: map[string]Entry{}}
	des, _ := os.ReadDir(dir)
	for _, de := range des {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, de.Name()))
		if err != nil {
			continue
		}
		var e Entry
		if json.Unmarshal(b, &e) == nil && e.ID != "" {
			idx.entries[e.ID] = e
		}
	}
	return idx, nil
}

// SetRefGuard installs the guard that tells the collector which blobs the
// journal still names. The host owns that knowledge (the journal is its
// event log), so the module asks instead of guessing. A nil guard removes the
// protection — the collector then behaves exactly as it did before.
func (i *Index) SetRefGuard(g RefGuard) {
	i.mu.Lock()
	i.guard = g
	i.mu.Unlock()
}

// protectedRefs asks the guard which of the candidate refs are still named by
// the journal. A nil guard protects nothing.
func (i *Index) protectedRefs(candidates []string) map[string]bool {
	i.mu.Lock()
	g := i.guard
	i.mu.Unlock()
	if g == nil || len(candidates) == 0 {
		return nil
	}
	return g.ReferencedRefs(candidates)
}

// PutEntry stores data in the blob store and records a metadata Entry. The caller
// supplies createdMs (the daemon stamps wall-clock; tests pass a fixed value) so
// the entry's sort key is explicit. The Entry's ID/Ref/Size are filled in here.
func (i *Index) PutEntry(meta Entry, data []byte, createdMs int64) (Entry, error) {
	ref, err := i.store.Put(data)
	if err != nil {
		return Entry{}, err
	}
	meta.ID = "art-" + ulid.New()
	meta.Ref = ref
	meta.Size = int64(len(data))
	meta.CreatedMS = createdMs
	i.mu.Lock()
	i.entries[meta.ID] = meta
	i.mu.Unlock()
	if err := i.writeMeta(meta); err != nil {
		return Entry{}, err
	}
	return meta, nil
}

// IndexRef records a metadata Entry for an ALREADY-stored blob (by content ref),
// without re-storing the bytes — used to index outputs the agent offloaded itself
// (the tool.result raw_ref). The ref must exist in the blob store. Size is filled
// from the store when meta.Size is 0.
func (i *Index) IndexRef(ref string, meta Entry, createdMs int64) (Entry, error) {
	ok, err := i.store.Has(ref)
	if err != nil {
		return Entry{}, err
	}
	if !ok {
		return Entry{}, ErrNotFound
	}
	meta.ID = "art-" + ulid.New()
	meta.Ref = ref
	meta.CreatedMS = createdMs
	if meta.Size == 0 {
		if sz, serr := i.store.Size(ref); serr == nil {
			meta.Size = sz
		}
	}
	i.mu.Lock()
	i.entries[meta.ID] = meta
	i.mu.Unlock()
	if err := i.writeMeta(meta); err != nil {
		return Entry{}, err
	}
	return meta, nil
}

func (i *Index) writeMeta(e Entry) error {
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicfile.WriteFile(filepath.Join(i.dir, e.ID+".json"), b, 0o600); err != nil {
		return fmt.Errorf("artifact index: write: %w", err)
	}
	return nil
}

// List returns the entries matching f, newest first (tie-broken by id desc).
func (i *Index) List(f Filter) []Entry {
	i.mu.Lock()
	out := make([]Entry, 0, len(i.entries))
	for _, e := range i.entries {
		if f.Kind != "" && e.Kind != f.Kind {
			continue
		}
		if f.Source != "" && e.Source != f.Source {
			continue
		}
		if f.Corr != "" && e.Corr != f.Corr {
			continue
		}
		out = append(out, e)
	}
	i.mu.Unlock()
	sort.Slice(out, func(a, b int) bool {
		if out[a].CreatedMS != out[b].CreatedMS {
			return out[a].CreatedMS > out[b].CreatedMS
		}
		return out[a].ID > out[b].ID
	})
	return out
}

// StaleEntries returns the entries created strictly before olderThanMs, OLDEST
// first — the collector's candidates for removal (M845). Entries with no creation
// time (CreatedMS<=0) are never considered stale, so a malformed entry isn't
// reaped by accident.
//
// The candidates are the COLLECTABLE ones: an entry whose blob the journal
// still names (a tool.result's raw_ref) is not a candidate at all, so the
// dry-run and the reap agree and neither promises bytes it will not free.
func (i *Index) StaleEntries(olderThanMs int64) []Entry {
	i.mu.Lock()
	out := make([]Entry, 0)
	for _, e := range i.entries {
		if e.CreatedMS > 0 && e.CreatedMS < olderThanMs {
			out = append(out, e)
		}
	}
	i.mu.Unlock()
	sort.Slice(out, func(a, b int) bool {
		if out[a].CreatedMS != out[b].CreatedMS {
			return out[a].CreatedMS < out[b].CreatedMS // oldest first
		}
		return out[a].ID < out[b].ID
	})
	refs := make([]string, 0, len(out))
	for _, e := range out {
		refs = append(refs, e.Ref)
	}
	protected := i.protectedRefs(refs)
	if len(protected) == 0 {
		return out
	}
	collectable := make([]Entry, 0, len(out))
	for _, e := range out {
		if !protected[e.Ref] {
			collectable = append(collectable, e)
		}
	}
	return collectable
}

// Collect deletes the entries created before olderThanMs and returns how many
// were removed and the summed bytes of their entries (the blob is GC'd by the
// delete only when no surviving entry references it, so freed disk may be ≤
// bytes). The collector (M845) runs this after the operator confirms a dry-run.
//
// The candidates come from StaleEntries, so the journal-referenced artifacts are
// already excluded — the guard is asked once for the whole batch, not once per
// entry.
func (i *Index) Collect(olderThanMs int64) (collected int, bytes int64) {
	for _, e := range i.StaleEntries(olderThanMs) {
		if err := i.deleteEntry(e.ID); err == nil {
			collected++
			bytes += e.Size
		}
	}
	return collected, bytes
}

// Get returns the Entry for id.
func (i *Index) Get(id string) (Entry, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	e, ok := i.entries[id]
	return e, ok
}

// Bytes returns the blob bytes (and Entry) for an entry id, re-verified by the
// store against the content ref.
func (i *Index) Bytes(id string) ([]byte, Entry, error) {
	e, ok := i.Get(id)
	if !ok {
		return nil, Entry{}, ErrNotFound
	}
	b, err := i.store.Get(e.Ref)
	return b, e, err
}

// Count returns how many entries are indexed.
func (i *Index) Count() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return len(i.entries)
}

// Delete removes the entry and its metadata file. The underlying blob is
// garbage-collected only when no remaining entry references the same ref (the
// content-address dedup means one blob may back several arrivals) — and never
// when the RefGuard says the journal still names those bytes: the deletion is
// refused with ErrStillReferenced instead, because a raw_ref pointing at
// deleted bytes would be a dangling promise.
func (i *Index) Delete(id string) error {
	i.mu.Lock()
	e, ok := i.entries[id]
	i.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	if i.protectedRefs([]string{e.Ref})[e.Ref] {
		return ErrStillReferenced
	}
	return i.deleteEntry(id)
}

// deleteEntry removes the entry and collects its blob when nothing else
// references it. The caller holds no lock; the guard check already happened.
func (i *Index) deleteEntry(id string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	e, ok := i.entries[id]
	if !ok {
		return ErrNotFound
	}
	// Do not forget an entry or collect its blob until metadata removal succeeds.
	// Keep ownership under the index lock across the filesystem operation.
	if err := os.Remove(filepath.Join(i.dir, id+".json")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("artifact index: remove metadata %s: %w", id, err)
	}
	delete(i.entries, id)
	stillUsed := false
	for _, other := range i.entries {
		if other.Ref == e.Ref {
			stillUsed = true
			break
		}
	}
	if !stillUsed {
		_ = os.Remove(i.store.pathFor(e.Ref))
	}
	return nil
}
