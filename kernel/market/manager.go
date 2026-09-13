// SPDX-License-Identifier: MIT

// Package market is the marketplace manager for skill/MCP packs.
//
// This file holds the manager surface: the Library / SkillInstaller /
// MCPInstaller / skillQuarantiner / mcpRemover contracts + the Manager +
// Config types + NewManager + the source-registry operations
// (Sources + AddSource + RemoveSource + Sync).
// The listing + install + uninstall path lives in manager_ops.go.
//
// Extracted from manager.go during the Day-208 god-file split.
// Public API unchanged.
package market

import (
	"context"
	"fmt"

	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/skill"
)

// Library is a source of packs — the built-in Official marketplace and, later,
// synced remotes. Kept an interface so the seed (plugins/builtinmarket) and the
// remote sync layer plug in without market importing them.
type Library interface {
	Marketplaces() []Marketplace
	// ResolvePack returns the full pack. marketplace may be "" to search all;
	// version may be "" for the catalogued version.
	ResolvePack(marketplace, name, version string) (Pack, error)
}

// SkillInstaller materializes a pack's skills (satisfied by *skill.Forge).
type SkillInstaller interface {
	Create(corr string, spec skill.CreateSpec) (skill.Skill, bool, error)
	Promote(corr, id string) (skill.Status, error)
}

// MCPInstaller registers a pack's MCP servers (satisfied by *runtime.Kernel).
type MCPInstaller interface {
	AddMCPServer(corr string, srv mcp.Server) (mcp.Server, error)
}

// skillQuarantiner / mcpRemover are OPTIONAL reverse operations used by Uninstall.
// They're asserted at runtime so a Manager can be built without them (tests).
type skillQuarantiner interface {
	Quarantine(corr, id, reason string) error
}
type mcpRemover interface {
	RemoveMCPServer(corr, ref string) (bool, error)
}

// Event is a progress frame emitted during install/uninstall (streamed to the UI
// and journaled), mirroring the toolbox install stream.
type Event struct {
	Stage  string `json:"stage"` // vet | skill | mcp | tool | done
	Name   string `json:"name,omitempty"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// Manager ties a Library (catalogue) to the Store (provenance) and the live
// subsystems that execute capabilities (Forge, MCP). It is the single entry the
// control plane and CLI drive.
type Manager struct {
	lib    Library
	store  *Store
	skills SkillInstaller
	mcp    MCPInstaller
	now    func() int64 // unix ms
	verify VerifyFunc   // optional Ed25519 verification; nil = unsigned-allowed
	syncer *Syncer      // optional remote-sync engine; nil = offline-only
}

// VerifyFunc verifies a pack's signature. Returns (signed, error): signed=true
// means a valid signature was present and checked; signed=false means unsigned
// (allowed, but flagged). A non-nil error means a present signature FAILED — the
// install is refused.
type VerifyFunc func(p Pack) (signed bool, err error)

// Config wires a Manager. skills/mcp may be nil in tests that only browse.
type Config struct {
	Library Library
	Store   *Store
	Skills  SkillInstaller
	MCP     MCPInstaller
	Now     func() int64
	Verify  VerifyFunc
	Syncer  *Syncer
}

// NewManager builds a Manager.
func NewManager(cfg Config) *Manager {
	now := cfg.Now
	if now == nil {
		now = func() int64 { return 0 } // tests inject; daemon sets a real clock
	}
	return &Manager{lib: cfg.Library, store: cfg.Store, skills: cfg.Skills, mcp: cfg.MCP, now: now, verify: cfg.Verify, syncer: cfg.Syncer}
}

// Sources lists the configured remote marketplaces.
func (m *Manager) Sources() ([]Source, error) { return m.store.Sources() }

// AddSource registers (or updates) a remote marketplace source. It does not
// fetch — call Sync afterwards. name defaults from the URL host when empty.
func (m *Manager) AddSource(name, rawURL, pubKey string) (Source, error) {
	if name == "" {
		name = deriveSourceName(rawURL)
	}
	src := Source{Name: name, URL: rawURL, PubKey: pubKey, AddedMS: m.now()}
	if err := m.store.AddSource(src); err != nil {
		return Source{}, err
	}
	return src, nil
}

// RemoveSource drops a source and its cached catalogue.
func (m *Manager) RemoveSource(name string) (bool, error) { return m.store.RemoveSource(name) }

// Sync fetches a source's catalogue into the cache (keep-last-good). With an
// empty name it syncs every configured source, returning one result each; a
// single source's failure does not abort the others.
func (m *Manager) Sync(ctx context.Context, name string) ([]SyncResult, error) {
	if m.syncer == nil {
		return nil, fmt.Errorf("market: remote sync is not enabled on this daemon")
	}
	srcs, err := m.store.Sources()
	if err != nil {
		return nil, err
	}
	var targets []Source
	if name == "" {
		targets = srcs
	} else {
		for _, s := range srcs {
			if s.Name == name {
				targets = append(targets, s)
			}
		}
		if len(targets) == 0 {
			return nil, fmt.Errorf("market: no source named %q", name)
		}
	}
	var out []SyncResult
	var firstErr error
	for _, s := range targets {
		res, serr := m.syncer.Sync(ctx, m.store, s, m.now())
		if serr != nil {
			if firstErr == nil {
				firstErr = serr
			}
			continue
		}
		out = append(out, res)
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, firstErr
}

