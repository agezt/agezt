// SPDX-License-Identifier: MIT

package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileNames under <BaseDir>/catalog/.
const (
	FileAPI    = "api.json"    // remote sync target
	FileLocal  = "local.json"  // auto-discovery target
	FileCustom = "custom.json" // operator overrides; wins
	FileMeta   = "meta.json"   // sync timestamps, source URL
)

// Meta is the small sidecar describing the most-recent sync. Lives in
// meta.json next to api.json so operators can see "how fresh is this?"
// from the filesystem without running a tool.
type Meta struct {
	APISyncedAt   time.Time `json:"api_synced_at,omitempty"`
	APISourceURL  string    `json:"api_source_url,omitempty"`
	APIBytes      int       `json:"api_bytes,omitempty"`
	LocalSyncedAt time.Time `json:"local_synced_at,omitempty"`
	LocalSource   string    `json:"local_source,omitempty"`
	ProviderCount int       `json:"provider_count,omitempty"`
	ModelCount    int       `json:"model_count,omitempty"`
}

// Store is the on-disk catalog directory.
type Store struct {
	Dir string
	// metaMu serializes the read-modify-write of meta.json. SaveAPI and SaveLocal
	// each load meta, mutate disjoint fields, and save; the control plane runs them
	// in separate goroutines, so without serialization a concurrent sync+discover
	// loses one side's update (last writer wins on the whole struct). (M478)
	metaMu sync.Mutex
}

// NewStore returns a Store rooted at dir. Creates the directory if
// needed on the first write; reads succeed against an empty dir
// (returning an empty Catalog).
func NewStore(dir string) *Store { return &Store{Dir: dir} }

// Load reads every present file under Dir, merging in the order
// api → local → custom (custom wins). Missing files are skipped
// silently — first-run with no sync yet returns an empty Catalog,
// not an error.
func (s *Store) Load() (*Catalog, error) {
	out := NewEmpty()
	for _, name := range []string{FileAPI, FileLocal, FileCustom} {
		path := filepath.Join(s.Dir, name)
		raw, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("catalog: read %s: %w", name, err)
		}
		c, err := ParseAPIFile(raw)
		if err != nil {
			return nil, fmt.Errorf("catalog: %s: %w", name, err)
		}
		out.Merge(c)
		out.Sources = append(out.Sources, name)
	}
	if meta, err := s.LoadMeta(); err == nil {
		out.SyncedAt = meta.APISyncedAt
	}
	return out, nil
}

// SaveAPI writes raw bytes (already validated as a models.dev-shaped
// catalog) to api.json and updates meta. Atomic via tmp+rename.
func (s *Store) SaveAPI(raw []byte, sourceURL string) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(s.Dir, FileAPI), raw, 0o644); err != nil {
		return err
	}
	return s.updateMeta(func(meta *Meta) {
		meta.APISyncedAt = time.Now().UTC()
		meta.APISourceURL = sourceURL
		meta.APIBytes = len(raw)
		if c, _ := ParseAPIFile(raw); c != nil {
			meta.ProviderCount = len(c.Providers)
			mc := 0
			for _, p := range c.Providers {
				mc += len(p.Models)
			}
			meta.ModelCount = mc
		}
	})
}

// SaveLocal writes the auto-discovered catalog fragment to local.json,
// preserving custom and api. Source identifies where the discovery
// came from (e.g. "ollama@http://localhost:11434").
func (s *Store) SaveLocal(c *Catalog, source string) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	raw, err := c.MarshalAPI()
	if err != nil {
		return fmt.Errorf("catalog: marshal local: %w", err)
	}
	if err := atomicWrite(filepath.Join(s.Dir, FileLocal), raw, 0o644); err != nil {
		return err
	}
	return s.updateMeta(func(meta *Meta) {
		meta.LocalSyncedAt = time.Now().UTC()
		meta.LocalSource = source
	})
}

// SaveCustom writes the operator-curated catalog fragment to custom.json,
// preserving api.json + local.json. custom.json wins the Load() merge, so a
// provider written here overrides any models.dev entry with the same id — this
// is how Quick Connect pins an exact base URL (e.g. a coding-plan endpoint).
// Atomic via tmp+rename; no meta update (custom has no sync timestamp).
func (s *Store) SaveCustom(c *Catalog) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	raw, err := c.MarshalAPI()
	if err != nil {
		return fmt.Errorf("catalog: marshal custom: %w", err)
	}
	return atomicWrite(filepath.Join(s.Dir, FileCustom), raw, 0o644)
}

// loadCustom reads custom.json alone (not the merged catalog), returning an
// empty catalog when the file is absent. Used to read-modify-write the custom
// layer without disturbing api/local.
func (s *Store) loadCustom() (*Catalog, error) {
	raw, err := os.ReadFile(filepath.Join(s.Dir, FileCustom))
	if errors.Is(err, os.ErrNotExist) {
		return NewEmpty(), nil
	}
	if err != nil {
		return nil, err
	}
	return ParseAPIFile(raw)
}

// UpsertCustomProvider inserts or replaces (by ID) one provider in custom.json,
// accumulating across calls. Returns whether the provider was newly added (vs
// replaced). The caller typically reloads the kernel afterward so the entry
// goes live without a restart.
func (s *Store) UpsertCustomProvider(p *Provider) (added bool, err error) {
	if p == nil || p.ID == "" {
		return false, fmt.Errorf("catalog: custom provider needs an id")
	}
	cur, err := s.loadCustom()
	if err != nil {
		return false, err
	}
	_, existed := cur.Providers[p.ID]
	cur.Providers[p.ID] = p
	if err := s.SaveCustom(cur); err != nil {
		return false, err
	}
	return !existed, nil
}

// LoadMeta returns the sidecar or a zero-valued Meta if absent.
func (s *Store) LoadMeta() (Meta, error) {
	var m Meta
	raw, err := os.ReadFile(filepath.Join(s.Dir, FileMeta))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("catalog: parse meta: %w", err)
	}
	return m, nil
}

// updateMeta performs a serialized read-modify-write of the meta sidecar: it loads
// the current meta under metaMu, applies fn, and writes it back, so concurrent
// SaveAPI/SaveLocal updates to disjoint fields don't lose each other.
func (s *Store) updateMeta(fn func(*Meta)) error {
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	m, _ := s.LoadMeta()
	fn(&m)
	return s.saveMetaLocked(m)
}

// SaveMeta writes the sidecar (serialized against concurrent meta updates).
func (s *Store) SaveMeta(m Meta) error {
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	return s.saveMetaLocked(m)
}

func (s *Store) saveMetaLocked(m Meta) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.Dir, FileMeta), raw, 0o644)
}

func (s *Store) ensureDir() error {
	return os.MkdirAll(s.Dir, 0o755)
}

// atomicWrite writes via a UNIQUE temp + rename so a crash mid-write leaves the
// previous file intact, and two concurrent writes to the same target can't race on
// a shared "<path>.tmp" (one renaming a half-written temp the other is still
// writing). (M478)
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
