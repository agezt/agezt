// SPDX-License-Identifier: MIT

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
	Stage  string `json:"stage"` // skill | mcp | tool | done
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

// Listing is a catalogue row joined with install state, for the UI/CLI.
type Listing struct {
	MarketplaceEntry
	Marketplace string `json:"marketplace"`
	Builtin     bool   `json:"builtin"`
	Installed   bool   `json:"installed"`
	// UpdateAvailable is true when installed at a different version than catalogued.
	UpdateAvailable bool `json:"update_available,omitempty"`
}

// List returns every catalogued pack across marketplaces, joined with install
// state. Optional query filters by name/description/category/tags.
func (m *Manager) List(query string) ([]Listing, error) {
	installed, err := m.store.Installed()
	if err != nil {
		return nil, err
	}
	byName := map[string]InstalledPack{}
	for _, ip := range installed {
		byName[ip.Name] = ip
	}
	var out []Listing
	for _, mp := range m.lib.Marketplaces() {
		entries := append([]MarketplaceEntry(nil), mp.Packs...)
		sortEntries(entries)
		for _, e := range entries {
			if !matchesQuery(e, query) {
				continue
			}
			l := Listing{MarketplaceEntry: e, Marketplace: mp.Name, Builtin: mp.Builtin}
			if ip, ok := byName[e.Name]; ok {
				l.Installed = true
				l.UpdateAvailable = ip.Version != e.Version
			}
			out = append(out, l)
		}
	}
	return out, nil
}

// Show resolves a pack's full contents plus its install state.
func (m *Manager) Show(marketplace, name string) (Pack, InstalledPack, bool, error) {
	p, err := m.lib.ResolvePack(marketplace, name, "")
	if err != nil {
		return Pack{}, InstalledPack{}, false, err
	}
	ip, installed, err := m.store.InstalledByName(name)
	if err != nil {
		return Pack{}, InstalledPack{}, false, err
	}
	return p, ip, installed, nil
}

// Install materializes a pack: its skills into the Forge (promoted active), its
// MCP servers into the registry, and its tool requirements reported (never
// host-installed silently). emit (nil-safe) streams progress. Returns the
// recorded install. Idempotent: re-installing updates the record.
func (m *Manager) Install(corr, marketplace, name, version string, emit func(Event)) (InstalledPack, error) {
	send := func(e Event) {
		if emit != nil {
			emit(e)
		}
	}
	if m.skills == nil || m.mcp == nil {
		return InstalledPack{}, fmt.Errorf("market: install requires live skill+mcp subsystems")
	}
	p, err := m.lib.ResolvePack(marketplace, name, version)
	if err != nil {
		return InstalledPack{}, err
	}
	if err := p.Validate(); err != nil {
		return InstalledPack{}, err
	}

	// Trust: verify a present signature; unsigned is allowed but flagged.
	unsigned := true
	if m.verify != nil {
		signed, verr := m.verify(p)
		if verr != nil {
			return InstalledPack{}, fmt.Errorf("market: signature verification failed for %q: %w", p.Name, verr)
		}
		unsigned = !signed
	}

	rec := InstalledPack{
		Name:        p.Name,
		Version:     p.Version,
		Marketplace: marketplace,
		InstalledMS: m.now(),
		Unsigned:    unsigned,
	}

	// Skills → Forge.Create + promote to active (reuses the seed path's logic).
	for _, ps := range p.Skills {
		md, perr := skill.ParseSkillMD([]byte(ps.SkillMD))
		if perr != nil {
			send(Event{Stage: "skill", OK: false, Detail: perr.Error()})
			return rec, fmt.Errorf("market: parse skill in %q: %w", p.Name, perr)
		}
		sk, _, cerr := m.skills.Create(corr, skill.CreateSpec{
			Name:          md.Name,
			Description:   md.Description,
			Triggers:      md.Triggers,
			Body:          md.Body,
			ToolsRequired: md.ToolsRequired,
			Resources:     ps.Resources,
		})
		if cerr != nil {
			send(Event{Stage: "skill", Name: md.Name, OK: false, Detail: cerr.Error()})
			return rec, fmt.Errorf("market: install skill %q: %w", md.Name, cerr)
		}
		m.promoteToActive(corr, sk)
		rec.SkillIDs = append(rec.SkillIDs, sk.ID)
		send(Event{Stage: "skill", Name: md.Name, OK: true, Detail: "active"})
	}

	// MCP servers → registry (validated by AddMCPServer too).
	for _, srv := range p.MCPServers {
		added, aerr := m.mcp.AddMCPServer(corr, srv)
		if aerr != nil {
			send(Event{Stage: "mcp", Name: srv.Name, OK: false, Detail: aerr.Error()})
			return rec, fmt.Errorf("market: add mcp %q: %w", srv.Name, aerr)
		}
		rec.MCPServers = append(rec.MCPServers, added.Name)
		send(Event{Stage: "mcp", Name: added.Name, OK: true, Detail: "registered"})
	}

	// Tool requirements are reported, not host-installed (host exec needs consent).
	for _, t := range p.ToolRequirements {
		rec.ToolReqs = append(rec.ToolReqs, t)
		send(Event{Stage: "tool", Name: t, OK: true, Detail: "required — install in Toolbox"})
	}

	if err := m.store.RecordInstall(rec); err != nil {
		return rec, err
	}
	detail := fmt.Sprintf("%d skill(s), %d mcp, %d tool req", len(rec.SkillIDs), len(rec.MCPServers), len(rec.ToolReqs))
	if unsigned {
		detail += " · unsigned"
	}
	send(Event{Stage: "done", Name: p.Name, OK: true, Detail: detail})
	return rec, nil
}

func (m *Manager) promoteToActive(corr string, sk skill.Skill) {
	status := sk.Status
	for i := 0; i < 3 && status != skill.StatusActive; i++ {
		next, err := m.skills.Promote(corr, sk.ID)
		if err != nil || next == status {
			break
		}
		status = next
	}
}

// Uninstall reverses a pack's footprint via its recorded provenance: it
// quarantines the skills it installed and removes the MCP servers it added. It
// only touches what THIS pack created (best-effort; missing optional reverse
// APIs are skipped). Tool requirements are left alone (host tools are shared).
func (m *Manager) Uninstall(corr, name string, emit func(Event)) error {
	send := func(e Event) {
		if emit != nil {
			emit(e)
		}
	}
	rec, ok, err := m.store.InstalledByName(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("market: %q is not installed", name)
	}
	if q, ok := m.skills.(skillQuarantiner); ok {
		for _, id := range rec.SkillIDs {
			if qerr := q.Quarantine(corr, id, "market uninstall "+name); qerr != nil {
				send(Event{Stage: "skill", Name: id, OK: false, Detail: qerr.Error()})
			} else {
				send(Event{Stage: "skill", Name: id, OK: true, Detail: "quarantined"})
			}
		}
	}
	if r, ok := m.mcp.(mcpRemover); ok {
		for _, srv := range rec.MCPServers {
			if _, rerr := r.RemoveMCPServer(corr, srv); rerr != nil {
				send(Event{Stage: "mcp", Name: srv, OK: false, Detail: rerr.Error()})
			} else {
				send(Event{Stage: "mcp", Name: srv, OK: true, Detail: "removed"})
			}
		}
	}
	if _, err := m.store.RemoveInstall(name); err != nil {
		return err
	}
	send(Event{Stage: "done", Name: name, OK: true, Detail: "uninstalled"})
	return nil
}
