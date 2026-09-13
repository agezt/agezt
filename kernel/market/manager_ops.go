// SPDX-License-Identifier: MIT
//
// market listing + install + uninstall path: Listing struct + List + Show
// (the catalogue accessors) + Install + promoteToActive (the install path)
// + Uninstall.
// Extracted from manager.go during the Day-208 god-file split.
// Public API unchanged.
package market

import (
	"fmt"

	"github.com/agezt/agezt/kernel/skill"
)

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
				// Strictly newer, not merely different: string inequality would
				// offer a DOWNGRADE (or a prerelease of an installed release)
				// as an update the moment the catalogue rolled back.
				l.UpdateAvailable = CompareVersions(e.Version, ip.Version) > 0
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

	// Refuse a silent downgrade. The install record is keyed by pack name, so
	// recording an older pack over an installed newer one rewrites provenance
	// with no error anywhere — and List, which orders versions, would then
	// report nothing to update. The agent tool always installs the catalogued
	// version, so a marketplace that rolled back would downgrade every
	// agent-initiated install invisibly. Uninstall reverses the installed
	// footprint first; that is the supported rollback path.
	if ip, ok, err := m.store.InstalledByName(p.Name); err != nil {
		return InstalledPack{}, err
	} else if ok && CompareVersions(p.Version, ip.Version) < 0 {
		return InstalledPack{}, fmt.Errorf("market: %s is installed at %s; refusing downgrade to %s (uninstall first)", p.Name, ip.Version, p.Version)
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

	// Security review — informational, never a wall (default-allow): the report
	// streams to the operator and its verdict is recorded in provenance.
	vet := VetPack(p)
	send(Event{Stage: "vet", Name: p.Name, OK: vet.Verdict != VerdictDanger, Detail: vet.Summary()})

	rec := InstalledPack{
		Name:        p.Name,
		Version:     p.Version,
		Marketplace: marketplace,
		InstalledMS: m.now(),
		Unsigned:    unsigned,
		VetVerdict:  vet.Verdict,
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
