// SPDX-License-Identifier: MIT

package marketapi

import "context"

// Listing is a catalogue row joined with install state, for the UI/CLI.
type Listing struct {
	MarketplaceEntry
	Marketplace string `json:"marketplace"`
	Builtin     bool   `json:"builtin"`
	Installed   bool   `json:"installed"`
	// UpdateAvailable is true only when the catalogued version is strictly
	// NEWER than the installed one (semver precedence, see CompareVersions) —
	// a rolled-back catalogue or a prerelease of an installed release must not
	// offer a downgrade as an update.
	UpdateAvailable bool `json:"update_available,omitempty"`
}

// InstalledPack is the recorded footprint of one installed pack: what it
// materialized (skills, MCP servers), what it asked for (CLI tools) and how it
// was reviewed (the security verdict at install time). It is the install's
// provenance — "what did I let in, knowing what?".
type InstalledPack struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Marketplace string   `json:"marketplace"`
	InstalledMS int64    `json:"installed_ms"`
	SkillIDs    []string `json:"skill_ids,omitempty"`
	MCPServers  []string `json:"mcp_servers,omitempty"` // server names this pack added
	ToolReqs    []string `json:"tool_reqs,omitempty"`
	Unsigned    bool     `json:"unsigned,omitempty"`
	// VetVerdict records the security review's verdict at install time
	// (clean|caution|danger) — provenance for "what did I let in, knowing what?".
	VetVerdict string `json:"vet_verdict,omitempty"`
}

// Event is one progress step of an install or an uninstall: the stage it
// reached, what it was about, whether it succeeded, and the detail an operator
// reads. The steps stream to the caller so a long install reports its way
// through instead of going quiet.
type Event struct {
	Stage  string `json:"stage"` // vet | skill | mcp | tool | done
	Name   string `json:"name,omitempty"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// Manager is the port through which the marketplace is driven: the catalogue
// is read, a pack is installed or uninstalled, and remote sources are added,
// removed and synced.
//
// The port is EIGHT verbs wide because its holders need all eight: the control
// plane reads the catalogue and the sources (List, Show, Sources) and drives
// the lifecycle (InstallContext, UninstallContext, AddSource, RemoveSource,
// Sync). It is a host-facing port — a plugin never holds it, so the widths
// that matter for plugins (the Library port) stay where they are.
//
// The rows it carries live here, not in the module: the built-in "Official"
// catalogue is a PLUGIN, and a plugin may reach the contract and the platform
// layers and nothing else. The manager that implements this port stays in the
// module (kernel/market); the module keeps the names its own callers use as
// aliases, so there is one row and one spelling of it.
type Manager interface {
	// List returns every catalogued pack across marketplaces, joined with
	// install state. An empty query lists all.
	List(query string) ([]Listing, error)
	// Show resolves one pack's full contents plus its install state.
	Show(marketplace, name string) (Pack, InstalledPack, bool, error)
	// Sources returns the configured remote sources (empty if none).
	Sources() ([]Source, error)
	// InstallContext materializes a pack: its skills into the Forge, its MCP
	// servers into the registry. emit (nil-safe) streams progress and may stop
	// the install by returning an error.
	InstallContext(ctx context.Context, corr, marketplace, name, version string, emit func(Event) error) (InstalledPack, error)
	// UninstallContext reverses a pack's recorded footprint.
	UninstallContext(ctx context.Context, corr, name string, emit func(Event) error) error
	// AddSource registers a remote marketplace URL.
	AddSource(name, rawURL, pubKey string) (Source, error)
	// RemoveSource drops a remote marketplace; it reports whether it was there.
	RemoveSource(name string) (bool, error)
	// Sync fetches a source's index and packs into the cache.
	Sync(ctx context.Context, name string) ([]SyncResult, error)
}
