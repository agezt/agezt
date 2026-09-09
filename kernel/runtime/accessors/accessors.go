// SPDX-License-Identifier: MIT

package accessors

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/agentgw"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/datalake"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/reflect"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/worldmodel"
	"github.com/agezt/agezt/kernel/runtime/types"
)

// Accessor is the read-side kernel companion to lifecycle.Manager.
// Day 15: 11 store getters (Day 13) + 7 live reads (Day 14) + 10
// Standing/Roster CRUD helpers (Day 15) = 28 methods total.
//
// Construction: one Accessor per *Kernel, initialised in Open()
// and shared for the kernel's whole lifetime.
type Accessor struct {
	k KernelAPI
}

// New constructs the Accessor. k is the host kernel; the Accessor
// is a stateless value around it.
func New(k KernelAPI) *Accessor { return &Accessor{k: k} }

// ---- Day 13: store getters ----

func (a *Accessor) Journal() *journal.Journal { return a.k.Journal() }
func (a *Accessor) Bus() *bus.Bus             { return a.k.Bus() }
func (a *Accessor) State() *state.FileStore   { return a.k.State() }
func (a *Accessor) Edict() *edict.Engine       { return a.k.Edict() }
func (a *Accessor) Warden() warden.Engine     { return a.k.Warden() }
func (a *Accessor) Approvals() *approval.Registry { return a.k.Approvals() }
func (a *Accessor) Scheduler() *scheduler.Executor { return a.k.Scheduler() }
func (a *Accessor) Provider() agent.Provider  { return a.k.Provider() }
func (a *Accessor) Memory() *memory.Manager   { return a.k.Memory() }
func (a *Accessor) AgentGateway() *agentgw.Gateway { return a.k.AgentGateway() }
func (a *Accessor) Schedules() *cadence.Store { return a.k.Schedules() }

// ---- Day 14: live reads ----

func (a *Accessor) Tools() map[string]agent.Tool { return a.k.Tools() }
func (a *Accessor) World() *worldmodel.Graph   { return a.k.World() }
func (a *Accessor) Forge() *skill.Forge         { return a.k.Forge() }
func (a *Accessor) StartTime() time.Time        { return a.k.StartTime() }
func (a *Accessor) ConfigCenter() *configcenter.Center { return a.k.ConfigCenter() }
func (a *Accessor) MaxDuration() time.Duration  { return a.k.MaxDuration() }
func (a *Accessor) Reflect() *reflect.Engine    { return a.k.Reflect() }

// ---- Day 16: store + configMu-free reads / writes ----

// Market returns the capability marketplace manager (skill/MCP/tool
// packs). It is nil until the daemon wires it via SetMarket (the
// built-in catalogue is a plugin the kernel must not import, so it
// is injected from cmd/agezt).
func (a *Accessor) Market() *market.Manager { return a.k.Market() }

// SetMarket injects the marketplace manager (from cmd/agezt, with
// the built-in Official library + this kernel's Forge/MCP as the
// install targets).
func (a *Accessor) SetMarket(m *market.Manager) { a.k.SetMarket(m) }

// BaseDir returns the kernel's base directory — the root under
// which journal/, state/, runtime/, catalog/, and vault data live.
func (a *Accessor) BaseDir() string { return a.k.BaseDir() }

// ArtifactIndex returns the metadata index over the blob store
// (M822) — the browsable/deletable per-arrival entries (inbound
// images, tool outputs) the file manager and inbound-image
// persistence use.
func (a *Accessor) ArtifactIndex() *artifact.Index { return a.k.ArtifactIndex() }

// DataLake returns the Personal Data Lake (M834) — the file-based
// structured collections agents build and share, surfaced by the
// `db` tool and the Web UI.
func (a *Accessor) DataLake() *datalake.Lake { return a.k.DataLake() }

// ---- Day 18a: configMu-guarded live mutators + read helpers ----

// Model returns the live default model name (M816). Seeded from
// cfg.Model at Open and hot-swapped via SetModel. The Accessor
// reads it under the host's configMu.
func (a *Accessor) Model() string {
	mu := a.k.ConfigMu()
	mu.Lock()
	defer mu.Unlock()
	return a.k.Model()
}

// SetModel replaces the live default model id. The next run picks
// it up — no restart.
func (a *Accessor) SetModel(m string) {
	mu := a.k.ConfigMu()
	mu.Lock()
	defer mu.Unlock()
	a.k.SetModel(m)
}

// System returns the live daemon default identity prompt (M710).
func (a *Accessor) System() string {
	mu := a.k.ConfigMu()
	mu.Lock()
	defer mu.Unlock()
	return a.k.System()
}

// SetSystem replaces the live daemon default identity prompt.
func (a *Accessor) SetSystem(s string) {
	mu := a.k.ConfigMu()
	mu.Lock()
	defer mu.Unlock()
	a.k.SetSystem(s)
}

// SetCouncilMembers replaces the live default Council of Elders
// membership (M839). The next council convening picks it up.
func (a *Accessor) SetCouncilMembers(members func() []types.CouncilMember) {
	mu := a.k.ConfigMu()
	mu.Lock()
	defer mu.Unlock()
	a.k.SetCouncilMembers(members)
}

// CouncilMembers returns the live default Council of Elders
// membership. Read under configMu so the read pairs with the
// Set* helpers.
func (a *Accessor) CouncilMembers() func() []types.CouncilMember {
	mu := a.k.ConfigMu()
	mu.Lock()
	defer mu.Unlock()
	return a.k.CouncilMembers()
}

// SubAgentLimits reports the effective delegation-governance
// ceilings (M46–M48) for `agt status` (M49). Read-only.
func (a *Accessor) SubAgentLimits() types.SubAgentLimits {
	return a.k.SubAgentLimits()
}

// Plugins returns the external-plugin manifest the daemon
// supplied at Open(). Read-only — callers must not mutate the
// slice.
func (a *Accessor) Plugins() []types.PluginInfo { return a.k.Plugins() }

// ---- Day 19: catalog + reload + loop + plan ----

// Catalog returns the currently-loaded provider/model catalog.
// Read under configMu (Day 19). The returned pointer is the live
// snapshot; callers should treat it as read-only.
func (a *Accessor) Catalog() *catalog.Catalog {
	mu := a.k.ConfigMu()
	mu.Lock()
	defer mu.Unlock()
	return a.k.Catalog()
}

// CatalogStore returns the on-disk store backing the catalog,
// so the control plane can drive `agt catalog sync` writes.
func (a *Accessor) CatalogStore() *catalog.Store { return a.k.CatalogStore() }

// Reload refreshes both the catalog snapshot AND the live provider
// registry. Catalog reload is always performed; provider rebuild
// runs when Config.OnReload is non-nil. Returns (catalog,
// providersReloaded, err).
func (a *Accessor) Reload() (*catalog.Catalog, bool, error) { return a.k.Reload() }

// ReloadCatalog re-reads catalog files from disk and re-installs
// the snapshot into the Governor.
func (a *Accessor) ReloadCatalog() (*catalog.Catalog, error) { return a.k.ReloadCatalog() }

// LoopRunner returns a closure suitable for scheduler.LoopNode.Runner.
// The closure drives one agent.Run end-to-end via the kernel's
// configured Provider/Tools/Policy hook.
func (a *Accessor) LoopRunner() scheduler.LoopRunner { return a.k.LoopRunner() }

// RunPlan executes a pre-built Plan through the kernel's scheduler.
// Honors Halt: refuses to start when halted; in-flight nodes are
// cancelled when Halt is called mid-plan.
func (a *Accessor) RunPlan(ctx context.Context, plan scheduler.Plan, planID string) (*scheduler.PlanResult, error) {
	return a.k.RunPlan(ctx, plan, planID)
}

// ---- Day 15: Standing / Roster CRUD ----

// AddStanding validates and persists a standing order, journaling
// standing.created so the lifecycle is auditable (SPEC-16 §4).
func (a *Accessor) AddStanding(o standing.Order) (standing.Order, error) {
	saved, err := a.k.Standing().Add(o)
	if err != nil {
		return standing.Order{}, err
	}
	_, _ = a.k.PublishBusEvent(event.Spec{
		Subject: "standing." + saved.ID, Kind: event.KindStandingCreated, Actor: "standing",
		Payload: map[string]any{"id": saved.ID, "name": saved.Name, "triggers": len(saved.Triggers)},
	})
	return saved, nil
}

// SetStandingEnabled pauses/resumes a standing order, journaling
// standing.updated.
func (a *Accessor) SetStandingEnabled(id string, enabled bool) (standing.Order, error) {
	o, err := a.k.Standing().SetEnabled(id, enabled)
	if err != nil {
		return standing.Order{}, err
	}
	state := "paused"
	if enabled {
		state = "resumed"
	}
	_, _ = a.k.PublishBusEvent(event.Spec{
		Subject: "standing." + id, Kind: event.KindStandingUpdated, Actor: "standing",
		Payload: map[string]any{"id": id, "name": o.Name, "enabled": enabled, "action": state},
	})
	return o, nil
}

// UpdateStanding edits a standing order's mutable fields via mutate,
// journaling standing.updated (action "edited") on success.
func (a *Accessor) UpdateStanding(id string, mutate func(*standing.Order)) (standing.Order, bool, error) {
	o, err := a.k.Standing().Update(id, mutate)
	if errors.Is(err, standing.ErrNotFound) {
		return standing.Order{}, false, nil
	}
	if err != nil {
		return standing.Order{}, false, err
	}
	_, _ = a.k.PublishBusEvent(event.Spec{
		Subject: "standing." + id, Kind: event.KindStandingUpdated, Actor: "standing",
		Payload: map[string]any{"id": id, "name": o.Name, "action": "edited"},
	})
	return o, true, nil
}

// RemoveStanding deletes a standing order, journaling
// standing.removed when it existed.
func (a *Accessor) RemoveStanding(id string) (bool, error) {
	o, _ := a.k.Standing().Get(id)
	ok, err := a.k.Standing().Remove(id)
	if err != nil {
		return false, err
	}
	if ok {
		_, _ = a.k.PublishBusEvent(event.Spec{
			Subject: "standing." + id, Kind: event.KindStandingRemoved, Actor: "standing",
			Payload: map[string]any{"id": id, "name": o.Name},
		})
	}
	return ok, nil
}

// AddProfile validates and persists a named agent profile, journaling
// roster.created so the agent's birth is auditable.
func (a *Accessor) AddProfile(p roster.Profile) (roster.Profile, error) {
	saved, err := a.k.Roster().Add(p)
	if err != nil {
		return roster.Profile{}, err
	}
	_, _ = a.k.PublishBusEvent(event.Spec{
		Subject: "roster." + saved.Slug, Kind: event.KindRosterCreated, Actor: "roster",
		Payload: map[string]any{"id": saved.ID, "slug": saved.Slug, "name": saved.Name, "model": saved.Model},
	})
	return saved, nil
}

// SetProfileEnabled pauses/resumes an agent profile, journaling
// roster.updated.
func (a *Accessor) SetProfileEnabled(ref string, enabled bool) (roster.Profile, error) {
	p, err := a.k.Roster().SetEnabled(ref, enabled)
	if err != nil {
		return roster.Profile{}, err
	}
	state := "paused"
	if enabled {
		state = "resumed"
	}
	_, _ = a.k.PublishBusEvent(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "enabled": enabled, "action": state},
	})
	return p, nil
}

// SetProfileRetired moves an agent to the graveyard (true) or revives
// it (false) by ref, journaling roster.updated. Retiring also pauses
// the agent so it stops firing (M846). A graveyard agent is excluded
// from delegation (runSubAgent).
func (a *Accessor) SetProfileRetired(ref string, retired bool, reason ...string) (roster.Profile, error) {
	p, err := a.k.Roster().SetRetired(ref, retired, reason...)
	if err != nil {
		return roster.Profile{}, err
	}
	action := "revived"
	if retired {
		action = "retired"
	}
	_, _ = a.k.PublishBusEvent(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "retired": retired, "reason": p.RetiredReason, "action": action},
	})
	return p, nil
}

// AgentImpact reports what depends on an agent before it is
// retired/removed (M846). The operator sees this in the retire
// confirmation so the "etkileri" are explicit. Returns the affected
// orders as "name (id)" strings, or nil when nothing references it.
func (a *Accessor) AgentImpact(slug string) []string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil
	}
	var out []string
	for _, o := range a.k.Standing().List() {
		if strings.EqualFold(strings.TrimSpace(o.Agent), slug) {
			name := o.Name
			if name == "" {
				name = o.ID
			}
			out = append(out, fmt.Sprintf("%s (%s)", name, o.ID))
		}
	}
	return out
}

// UpdateProfile edits a profile's mutable fields via mutate,
// journaling roster.updated (action "edited"). Returns false + nil
// error for an unknown ref.
func (a *Accessor) UpdateProfile(ref string, mutate func(*roster.Profile)) (roster.Profile, bool, error) {
	p, err := a.k.Roster().Update(ref, mutate)
	if errors.Is(err, roster.ErrNotFound) {
		return roster.Profile{}, false, nil
	}
	if err != nil {
		return roster.Profile{}, false, err
	}
	_, _ = a.k.PublishBusEvent(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "action": "edited"},
	})
	return p, true, nil
}

// RemoveProfile deletes an agent profile, journaling roster.removed
// when it existed. Shipped guardians (System) are protected from
// hard delete (M961): they are the daemon's own self-healing fleet.
// They can still be paused or retired.
func (a *Accessor) RemoveProfile(ref string) (bool, error) {
	if p, ok := a.k.Roster().Get(ref); ok && p.System {
		return false, fmt.Errorf("agent %q is a protected system guardian — pause or retire it instead of removing", p.Slug)
	}
	gone, ok, err := a.k.Roster().Remove(ref)
	if err != nil {
		return false, err
	}
	if ok {
		_, _ = a.k.PublishBusEvent(event.Spec{
			Subject: "roster." + gone.Slug, Kind: event.KindRosterRemoved, Actor: "roster",
			Payload: map[string]any{"id": gone.ID, "slug": gone.Slug, "name": gone.Name},
		})
	}
	return ok, nil
}
