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

	"github.com/agezt/agezt/kernel/ulid"
)

// Entry is one stored artifact's metadata — a single ARRIVAL. The bytes live in
// the content-addressed blob Store under Ref (deduped across arrivals); an Entry
// is per-arrival, so the same image sent twice is two Entries pointing at one
// blob. This is the queryable, deletable layer the file-manager (M823) and the
// inbound-image persistence (M822) build on, while Store stays a pure blob store.
type Entry struct {
	ID        string `json:"id"`
	Ref       string `json:"ref"`
	Name      string `json:"name,omitempty"`
	Mime      string `json:"mime,omitempty"`
	Kind      string `json:"kind,omitempty"`   // image | tool-output | file | …
	Source    string `json:"source,omitempty"` // telegram | slack | discord | run | …
	Sender    string `json:"sender,omitempty"`
	Corr      string `json:"corr,omitempty"`
	Size      int64  `json:"size"`
	CreatedMs int64  `json:"created_ms"`
	Caption   string `json:"caption,omitempty"`
}

// Filter narrows List; empty fields impose no constraint (exact match otherwise).
type Filter struct {
	Kind   string
	Source string
	Corr   string
}

// Index is a metadata sidecar over a blob Store. Entries persist as one JSON file
// each under <baseDir>/index/<id>.json; the blob shards (<aa>/<ref>) are
// untouched. An in-memory map (loaded at Open) backs queries; all access is
// mutex-guarded. Deleting an Entry garbage-collects its blob only when no other
// Entry still references that ref.
type Index struct {
	store   *Store
	dir     string
	mu      sync.Mutex
	entries map[string]Entry
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
	meta.CreatedMs = createdMs
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
	tmp := filepath.Join(i.dir, ".tmp-"+e.ID)
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("artifact index: write: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(i.dir, e.ID+".json")); err != nil {
		_ = os.Remove(tmp)
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
		if out[a].CreatedMs != out[b].CreatedMs {
			return out[a].CreatedMs > out[b].CreatedMs
		}
		return out[a].ID > out[b].ID
	})
	return out
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
// content-address dedup means one blob may back several arrivals).
func (i *Index) Delete(id string) error {
	i.mu.Lock()
	e, ok := i.entries[id]
	if !ok {
		i.mu.Unlock()
		return ErrNotFound
	}
	delete(i.entries, id)
	stillUsed := false
	for _, other := range i.entries {
		if other.Ref == e.Ref {
			stillUsed = true
			break
		}
	}
	i.mu.Unlock()

	_ = os.Remove(filepath.Join(i.dir, id+".json"))
	if !stillUsed {
		_ = os.Remove(i.store.pathFor(e.Ref)) // GC the now-orphaned blob
	}
	return nil
}
