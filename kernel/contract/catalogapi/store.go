// SPDX-License-Identifier: MIT

package catalogapi

import "time"

// Meta is the small sidecar describing the most-recent sync. Lives in
// meta.json next to api.json so operators can see "how fresh is this?"
// from the filesystem without running a tool.
//
// The row lives in the contract layer: the surfaces that report the catalog's
// freshness (the app layer's listing, the control plane's status) are above
// the module and may not name it.
type Meta struct {
	APISyncedAt   time.Time `json:"api_synced_at,omitempty"`
	APISourceURL  string    `json:"api_source_url,omitempty"`
	APIBytes      int       `json:"api_bytes,omitempty"`
	LocalSyncedAt time.Time `json:"local_synced_at,omitempty"`
	LocalSource   string    `json:"local_source,omitempty"`
	ProviderCount int       `json:"provider_count,omitempty"`
	ModelCount    int       `json:"model_count,omitempty"`
}

// Store is the port the daemon's catalog directory answers: the on-disk
// catalog (api.json + local.json + custom.json + the meta sidecar).
//
// The port lives in the contract layer because three layers above the module
// drive it and none of them may name the module's implementation:
//
//   - the kernel reloads the snapshot from it (ReloadCatalog);
//   - the app layer saves a synced or a discovered catalog into it;
//   - the control plane's roster/clock surfaces read its freshness.
//
// The daemon opens the directory and hands the kernel this port; a host that
// was given none holds a nil port and the WRITE surfaces answer "the catalog
// library is not available on this daemon" rather than writing into nothing.
//
// The port's width is what the layers above actually drive — **five** verbs.
// The module's own store has more (SaveCustom, SaveMeta); those stay on the
// module's type and are not part of the contract, because no layer above the
// module drives them.
type Store interface {
	// Load reads every present file, merging api → local → custom (custom
	// wins). Missing files are skipped silently — a first run with no sync
	// yet yields an empty catalog, not an error.
	Load() (*Catalog, error)
	// SaveAPI writes raw bytes (already validated as a models.dev-shaped
	// catalog) to api.json and updates the sidecar.
	SaveAPI(raw []byte, sourceURL string) error
	// SaveLocal writes an auto-discovered catalog fragment to local.json,
	// preserving custom and api. Source identifies where the discovery came
	// from (e.g. "ollama@http://localhost:11434").
	SaveLocal(c *Catalog, source string) error
	// UpsertCustomProvider inserts or replaces (by ID) one provider in
	// custom.json, accumulating across calls. The bool says whether the
	// provider was newly added (vs replaced).
	UpsertCustomProvider(p *Provider) (added bool, err error)
	// LoadMeta returns the sidecar, or a zero-valued Meta if it is absent.
	LoadMeta() (Meta, error)
}
