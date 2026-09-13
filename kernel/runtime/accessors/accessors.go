// SPDX-License-Identifier: MIT

// Accessor pass-throughs: the 30+ one-liner getters + the model/system
// accessors + the catalog + scheduler accessors + RunPlan. Every method
// here is a pure forward to the underlying KernelAPI — no business logic.
// The mutators (SetMarket + the standing/profile mutators) live in
// accessors_mutate.go.
// Extracted from accessors.go during the Day-207 god-file split.
// Public API unchanged.
package accessors

import (
	"context"
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
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/reflect"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/skill"
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
