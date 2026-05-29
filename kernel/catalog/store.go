// SPDX-License-Identifier: MIT

package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	meta, _ := s.LoadMeta()
	meta.APISyncedAt = time.Now().UTC()
	meta.APISourceURL = sourceURL
	meta.APIBytes = len(raw)
	c, _ := ParseAPIFile(raw)
	if c != nil {
		meta.ProviderCount = len(c.Providers)
		mc := 0
		for _, p := range c.Providers {
			mc += len(p.Models)
		}
		meta.ModelCount = mc
	}
	return s.SaveMeta(meta)
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
	meta, _ := s.LoadMeta()
	meta.LocalSyncedAt = time.Now().UTC()
	meta.LocalSource = source
	return s.SaveMeta(meta)
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

// SaveMeta writes the sidecar.
func (s *Store) SaveMeta(m Meta) error {
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

// atomicWrite writes via tmp + rename so a crash mid-write leaves the
// previous file intact rather than a truncated half-write.
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
