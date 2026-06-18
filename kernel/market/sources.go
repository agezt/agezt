// SPDX-License-Identifier: MIT

package market

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Source is a configured remote marketplace: a URL to its marketplace.json and
// an optional Ed25519 public key (hex) to verify the packs it serves. Sources
// live in <base>/market/sources.json; their fetched catalogues are cached under
// <base>/market/marketplaces/<name>/.
type Source struct {
	Name    string `json:"name"`             // local handle (kebab); also the cache dir name
	URL     string `json:"url"`              // marketplace.json URL
	PubKey  string `json:"pubkey,omitempty"` // optional hex Ed25519 key to verify this source's packs
	AddedMS int64  `json:"added_ms,omitempty"`
}

func (s *Store) sourcesPath() string     { return filepath.Join(s.dir, "sources.json") }
func (s *Store) marketplacesDir() string { return filepath.Join(s.dir, "marketplaces") }
func (s *Store) marketplaceDir(n string) string {
	return filepath.Join(s.marketplacesDir(), n)
}

// Sources returns the configured remote sources (empty if none).
func (s *Store) Sources() ([]Source, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadSources()
}

func (s *Store) loadSources() ([]Source, error) {
	data, err := os.ReadFile(s.sourcesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Source
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("market: parse sources.json: %w", err)
	}
	return out, nil
}

// AddSource upserts a remote source (keyed by name). The built-in "official"
// name is reserved so a remote can never shadow the offline catalogue.
func (s *Store) AddSource(src Source) error {
	if !nameRe.MatchString(src.Name) {
		return fmt.Errorf("market: source name must match %s", nameRe)
	}
	if src.Name == MarketplaceOfficial {
		return fmt.Errorf("market: %q is the reserved built-in marketplace name", MarketplaceOfficial)
	}
	if !isHTTPURL(src.URL) {
		return fmt.Errorf("market: source url must be http(s)")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.loadSources()
	if err != nil {
		return err
	}
	replaced := false
	for i := range list {
		if list[i].Name == src.Name {
			list[i] = src
			replaced = true
			break
		}
	}
	if !replaced {
		list = append(list, src)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return s.saveSources(list)
}

// RemoveSource drops a source and its cached catalogue. Returns whether it existed.
func (s *Store) RemoveSource(name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.loadSources()
	if err != nil {
		return false, err
	}
	out := list[:0]
	found := false
	for _, src := range list {
		if src.Name == name {
			found = true
			continue
		}
		out = append(out, src)
	}
	if !found {
		return false, nil
	}
	if err := s.saveSources(out); err != nil {
		return false, err
	}
	// Best-effort: drop the cached catalogue too.
	_ = os.RemoveAll(s.marketplaceDir(name))
	return true, nil
}

func (s *Store) saveSources(list []Source) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(s.sourcesPath(), data, 0o644)
}

// SaveMarketplace caches a synced marketplace's index plus every pack bundle,
// keep-last-good: it stages into a sibling temp dir and swaps it over the live
// one only after all writes succeed, so a failed sync never corrupts the cache.
func (s *Store) SaveMarketplace(name string, mp Marketplace, packs []Pack) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("market: marketplace name must match %s", nameRe)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.marketplacesDir(), 0o755); err != nil {
		return err
	}
	final := s.marketplaceDir(name)
	staging := final + ".staging"
	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(staging, "packs"), 0o755); err != nil {
		return err
	}
	for _, p := range packs {
		data, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(staging, "packs", p.Name+".json"), data, 0o644); err != nil {
			return err
		}
	}
	idx, err := json.MarshalIndent(mp, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(staging, "index.json"), idx, 0o644); err != nil {
		return err
	}
	// Swap: remove the old live dir then rename staging over it. The window is
	// tiny and local; far safer than overwriting files in place during a fetch.
	if err := os.RemoveAll(final); err != nil {
		return err
	}
	return os.Rename(staging, final)
}

// CachedMarketplaces reads every synced marketplace index from the cache.
func (s *Store) CachedMarketplaces() ([]Marketplace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.marketplacesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Marketplace
	for _, e := range entries {
		if !e.IsDir() || strings.HasSuffix(e.Name(), ".staging") {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(s.marketplaceDir(e.Name()), "index.json"))
		if rerr != nil {
			continue // a half-populated dir — skip rather than fail the whole list
		}
		var mp Marketplace
		if json.Unmarshal(data, &mp) == nil {
			out = append(out, mp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// CachedPack reads one pack bundle from a synced marketplace's cache.
func (s *Store) CachedPack(marketplace, name string) (Pack, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if marketplace == "" {
		return Pack{}, fmt.Errorf("market: a marketplace name is required to resolve a cached pack")
	}
	data, err := os.ReadFile(filepath.Join(s.marketplaceDir(marketplace), "packs", name+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return Pack{}, fmt.Errorf("market: pack %q not cached for marketplace %q", name, marketplace)
		}
		return Pack{}, err
	}
	var p Pack
	if err := json.Unmarshal(data, &p); err != nil {
		return Pack{}, fmt.Errorf("market: parse cached pack %q: %w", name, err)
	}
	return p, nil
}

// isHTTPURL is a cheap scheme guard; netguard screens the actual connection.
func isHTTPURL(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}
