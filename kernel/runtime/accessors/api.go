// SPDX-License-Identifier: MIT

package accessors

import (
	"context"
	"sync"
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

// KernelAPI is the surface the Accessor needs from its host kernel.
// Day 16 fourth slice: Day 13 + Day 14 + Day 15 + 5 new configMu-
// free reads / writes (Market, SetMarket, BaseDir, ArtifactIndex,
// DataLake). Mutators gated by configMu (Model/SetModel,
// System/SetSystem, SetCouncilMembers, ScheduleEngine/
// SetScheduleEngine) and type-coupled surface (SubAgentLimits,
// Plugins, Voice) land in subsequent commits — they need either
// a kernel/runtime/types shared-package extraction or a narrower
// accessor pattern that doesn't require the type at the sub-package
// boundary.
//
// *Kernel satisfies this interface implicitly via its public methods.
type KernelAPI interface {
	// Day 13 — store getters.
	Journal() *journal.Journal
	Bus() *bus.Bus
	State() *state.FileStore
	Edict() *edict.Engine
	Warden() warden.Engine
	Approvals() *approval.Registry
	Scheduler() *scheduler.Executor
	Provider() agent.Provider
	Memory() *memory.Manager
	AgentGateway() *agentgw.Gateway
	Schedules() *cadence.Store

	// Day 14 — live reads.
	Tools() map[string]agent.Tool
	World() *worldmodel.Graph
	Forge() *skill.Forge
	StartTime() time.Time
	ConfigCenter() *configcenter.Center
	MaxDuration() time.Duration
	Reflect() *reflect.Engine

	// Day 14 — live mutators (configMu-guarded).
	ConfigMu() *sync.Mutex
	ScheduleEngine() *cadence.Engine
	SetScheduleEngine(*cadence.Engine)

	// Day 15 — Standing / Roster stores + bus hook.
	Standing() *standing.Store
	Roster() *roster.Store
	PublishBusEvent(spec event.Spec) (*event.Event, error)

	// Day 16 — store + configMu-free reads / writes.
	Market() *market.Manager
	SetMarket(*market.Manager)
	BaseDir() string
	ArtifactIndex() *artifact.Index
	DataLake() *datalake.Lake

	// Day 18a — configMu-guarded live mutators + read helpers.
	// The Accessor honours the lock order by taking the host's
	// ConfigMu() before reading or writing the live field.
	Model() string
	SetModel(string)
	System() string
	SetSystem(string)
	SetCouncilMembers(func() []types.CouncilMember)
	CouncilMembers() func() []types.CouncilMember
	SubAgentLimits() types.SubAgentLimits
	Plugins() []types.PluginInfo

	// Day 19 — catalog + reload + loop + plan.
	Catalog() *catalog.Catalog
	CatalogStore() *catalog.Store
	Reload() (*catalog.Catalog, bool, error)
	ReloadCatalog() (*catalog.Catalog, error)
	LoopRunner() scheduler.LoopRunner
	RunPlan(ctx context.Context, plan scheduler.Plan, planID string) (*scheduler.PlanResult, error)
}
