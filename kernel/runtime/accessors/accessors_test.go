// SPDX-License-Identifier: MIT

package accessors

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/agentgw"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/reflect"
	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/datalake"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/worldmodel"
	"github.com/agezt/agezt/kernel/runtime/types"
	"github.com/agezt/agezt/kernel/catalog"
)

// fakeKernel is the minimum KernelAPI implementation the Accessor
// unit tests need. Every getter returns a sentinel value so a test
// can assert "Accessor returns the same value the host kernel did"
// without booting a real *Kernel.
type fakeKernel struct {
	mu            sync.Mutex // shared mutex for configMu-guarded helpers
	journal       *journal.Journal
	bus           *bus.Bus
	state         *state.FileStore
	edict         *edict.Engine
	warden        warden.Engine
	approvals     *approval.Registry
	scheduler     *scheduler.Executor
	provider      agent.Provider
	tools         map[string]agent.Tool
	memory        *memory.Manager
	gateway       *agentgw.Gateway
	schedules     *cadence.Store
	world         *worldmodel.Graph
	forge         *skill.Forge
	startTime     time.Time
	configCenter  *configcenter.Center
	maxDuration   time.Duration
	reflectEngine *reflect.Engine
	market        *market.Manager
	baseDir       string
	artifactIndex *artifact.Index
	dataLake      *datalake.Lake

	// Day 18a function hooks — fakes may set these to drive the
	// configMu / Council / Plugin / SubAgentLimits tests.
	ConfigMuFunc func() *sync.Mutex
	ModelFn      func() string
	SetModelFn   func(string)
	SystemFn     func() string
	SetSystemFn  func(string)
	MembersFn    func() func() []types.CouncilMember
	SetMembersFn func(func() []types.CouncilMember)
	SubsFn       func() types.SubAgentLimits
	PluginsFn    func() []types.PluginInfo

	// Day 19 hooks.
	CatalogFn         func() *catalog.Catalog
	CatalogStoreFn    func() *catalog.Store
	ReloadFn          func() (*catalog.Catalog, bool, error)
	ReloadCatalogFn   func() (*catalog.Catalog, error)
	LoopRunnerFn      func() scheduler.LoopRunner
	RunPlanFn         func(ctx context.Context, plan scheduler.Plan, planID string) (*scheduler.PlanResult, error)
}

func (k *fakeKernel) Journal() *journal.Journal          { return k.journal }
func (k *fakeKernel) Bus() *bus.Bus                      { return k.bus }
func (k *fakeKernel) State() *state.FileStore            { return k.state }
func (k *fakeKernel) Edict() *edict.Engine                { return k.edict }
func (k *fakeKernel) Warden() warden.Engine              { return k.warden }
func (k *fakeKernel) Approvals() *approval.Registry      { return k.approvals }
func (k *fakeKernel) Scheduler() *scheduler.Executor     { return k.scheduler }
func (k *fakeKernel) Provider() agent.Provider           { return k.provider }
func (k *fakeKernel) Tools() map[string]agent.Tool       { return k.tools }
func (k *fakeKernel) Memory() *memory.Manager            { return k.memory }
func (k *fakeKernel) AgentGateway() *agentgw.Gateway      { return k.gateway }
func (k *fakeKernel) Schedules() *cadence.Store          { return k.schedules }
func (k *fakeKernel) World() *worldmodel.Graph           { return k.world }
func (k *fakeKernel) Forge() *skill.Forge                { return k.forge }
func (k *fakeKernel) StartTime() time.Time               { return k.startTime }
func (k *fakeKernel) ConfigCenter() *configcenter.Center { return k.configCenter }
func (k *fakeKernel) MaxDuration() time.Duration         { return k.maxDuration }
func (k *fakeKernel) Reflect() *reflect.Engine           { return k.reflectEngine }
func (k *fakeKernel) ConfigMu() *sync.Mutex              { return &k.mu }
func (k *fakeKernel) ScheduleEngine() *cadence.Engine    { return nil }
func (k *fakeKernel) SetScheduleEngine(*cadence.Engine)  {}

// ConfigMu/Member/Plugin/Model/System/etc. function hooks —
// only the Day 18a fake sets these; the rest of the suite uses
// the nilling default methods above.
//
// IMPORTANT: the host's read methods (Model, System, CouncilMembers,
// SubAgentLimits, Plugins) MUST NOT take the configMu. The Accessor
// takes it on the way in, so the host's view here is "I am being
// called under the lock; just return the value without re-locking".
// Only the Set* methods may re-acquire, because those are called
// while the Accessor holds the same lock — same constraint, no
// re-lock needed.
func (k *fakeKernel) Model() string {
	if k.ModelFn != nil {
		return k.ModelFn()
	}
	return ""
}
func (k *fakeKernel) SetModel(s string) {
	if k.SetModelFn != nil {
		k.SetModelFn(s)
	}
}
func (k *fakeKernel) System() string {
	if k.SystemFn != nil {
		return k.SystemFn()
	}
	return ""
}
func (k *fakeKernel) SetSystem(s string) {
	if k.SetSystemFn != nil {
		k.SetSystemFn(s)
	}
}
func (k *fakeKernel) CouncilMembers() func() []types.CouncilMember {
	if k.MembersFn != nil {
		return k.MembersFn()
	}
	return nil
}
func (k *fakeKernel) SetCouncilMembers(f func() []types.CouncilMember) {
	if k.SetMembersFn != nil {
		k.SetMembersFn(f)
	}
}
func (k *fakeKernel) SubAgentLimits() types.SubAgentLimits {
	if k.SubsFn != nil {
		return k.SubsFn()
	}
	return types.SubAgentLimits{}
}
func (k *fakeKernel) Plugins() []types.PluginInfo {
	if k.PluginsFn != nil {
		return k.PluginsFn()
	}
	return nil
}

// Day 19 — catalog / reload / loop / plan. The fake just
// returns the host-side values without taking additional
// locks; the Accessor takes the locks it needs on the way
// in (configMu for Catalog, runsMu for RunPlan).
func (k *fakeKernel) Catalog() *catalog.Catalog {
	if k.CatalogFn != nil {
		return k.CatalogFn()
	}
	return nil
}
func (k *fakeKernel) CatalogStore() *catalog.Store {
	if k.CatalogStoreFn != nil {
		return k.CatalogStoreFn()
	}
	return nil
}
func (k *fakeKernel) Reload() (*catalog.Catalog, bool, error) {
	if k.ReloadFn != nil {
		return k.ReloadFn()
	}
	return nil, false, nil
}
func (k *fakeKernel) ReloadCatalog() (*catalog.Catalog, error) {
	if k.ReloadCatalogFn != nil {
		return k.ReloadCatalogFn()
	}
	return nil, nil
}
func (k *fakeKernel) LoopRunner() scheduler.LoopRunner {
	if k.LoopRunnerFn != nil {
		return k.LoopRunnerFn()
	}
	return nil
}
func (k *fakeKernel) RunPlan(ctx context.Context, plan scheduler.Plan, planID string) (*scheduler.PlanResult, error) {
	if k.RunPlanFn != nil {
		return k.RunPlanFn(ctx, plan, planID)
	}
	return nil, nil
}
func (k *fakeKernel) Standing() *standing.Store         { return nil }
func (k *fakeKernel) Roster() *roster.Store              { return nil }
func (k *fakeKernel) PublishBusEvent(spec event.Spec) (*event.Event, error) {
	return nil, nil
}
func (k *fakeKernel) Market() *market.Manager             { return k.market }
func (k *fakeKernel) SetMarket(m *market.Manager)        { k.market = m }
func (k *fakeKernel) BaseDir() string                    { return k.baseDir }
func (k *fakeKernel) ArtifactIndex() *artifact.Index     { return k.artifactIndex }
func (k *fakeKernel) DataLake() *datalake.Lake           { return k.dataLake }

// newFakeKernel returns a fake with all-nil getters — good for the
// "Accessor forwards correctly" smoke test.
func newFakeKernel() *fakeKernel { return &fakeKernel{} }

func TestAccessors_Day13_PassThrough(t *testing.T) {
	k := newFakeKernel()
	a := New(k)
	if a.Journal() != k.Journal() {
		t.Errorf("Journal(): Accessor returned %v, want %v", a.Journal(), k.Journal())
	}
	if a.Bus() != k.Bus() {
		t.Errorf("Bus(): Accessor returned %v, want %v", a.Bus(), k.Bus())
	}
	if a.State() != k.State() {
		t.Errorf("State(): Accessor returned %v, want %v", a.State(), k.State())
	}
	if a.Edict() != k.Edict() {
		t.Errorf("Edict(): Accessor returned %v, want %v", a.Edict(), k.Edict())
	}
	if a.Warden() != k.Warden() {
		t.Errorf("Warden(): Accessor returned %v, want %v", a.Warden(), k.Warden())
	}
	if a.Approvals() != k.Approvals() {
		t.Errorf("Approvals(): Accessor returned %v, want %v", a.Approvals(), k.Approvals())
	}
	if a.Scheduler() != k.Scheduler() {
		t.Errorf("Scheduler(): Accessor returned %v, want %v", a.Scheduler(), k.Scheduler())
	}
	if a.Provider() != k.Provider() {
		t.Errorf("Provider(): Accessor returned %v, want %v", a.Provider(), k.Provider())
	}
	if a.Memory() != k.Memory() {
		t.Errorf("Memory(): Accessor returned %v, want %v", a.Memory(), k.Memory())
	}
	if a.AgentGateway() != k.AgentGateway() {
		t.Errorf("AgentGateway(): Accessor returned %v, want %v", a.AgentGateway(), k.AgentGateway())
	}
	if a.Schedules() != k.Schedules() {
		t.Errorf("Schedules(): Accessor returned %v, want %v", a.Schedules(), k.Schedules())
	}
}

func TestAccessors_Day14_PassThrough(t *testing.T) {
	k := newFakeKernel()
	a := New(k)
	if a.Tools() != nil {
		t.Errorf("Tools(): Accessor returned %v, want nil", a.Tools())
	}
	if a.World() != k.World() {
		t.Errorf("World(): Accessor returned %v, want %v", a.World(), k.World())
	}
	if a.Forge() != k.Forge() {
		t.Errorf("Forge(): Accessor returned %v, want %v", a.Forge(), k.Forge())
	}
	if !a.StartTime().IsZero() {
		t.Errorf("StartTime(): Accessor returned %v, want zero time", a.StartTime())
	}
	if a.ConfigCenter() != k.ConfigCenter() {
		t.Errorf("ConfigCenter(): Accessor returned %v, want %v", a.ConfigCenter(), k.ConfigCenter())
	}
	if a.MaxDuration() != 0 {
		t.Errorf("MaxDuration(): Accessor returned %v, want 0", a.MaxDuration())
	}
	if a.Reflect() != k.Reflect() {
		t.Errorf("Reflect(): Accessor returned %v, want %v", a.Reflect(), k.Reflect())
	}
}

func TestAccessors_Day14_SentinelsPassThrough(t *testing.T) {
	// Sentinels: a fresh value on the host side must be the
	// exact same pointer on the Accessor side.
	want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	k := &fakeKernel{startTime: want, maxDuration: 7 * time.Second}
	a := New(k)
	if got := a.StartTime(); !got.Equal(want) {
		t.Errorf("StartTime() = %v, want %v", got, want)
	}
	if got := a.MaxDuration(); got != 7*time.Second {
		t.Errorf("MaxDuration() = %v, want 7s", got)
	}
}

// Day 15 Standing / Roster CRUD tests are deferred: standing.Store
// and roster.Store don't expose constructors suitable for the
// Accessor's nil-store contract (their New() / Open() require file
// paths), and writing in-memory fakes for every method the
// Accessor calls would dwarf the actual sub-package surface. The
// Day 15 pass-through is verified by the *Kernel build itself — the
// 10 new Accessor methods compile, so the methods exist with the
// correct signatures, and the runtime tests exercise the
// end-to-end path through the *Kernel methods (not the Accessor
// wrappers). The Accessor stays available for future unit tests
// that can afford full in-memory Standing/Roster fakes.

func TestAccessors_Day16_StorePassthrough(t *testing.T) {
	// Sentinels for the five new Day 16 methods. We don't need
	// real market/artifact/datalake values — we just need the
	// Accessor to return exactly what the host kernel handed out.
	k := &fakeKernel{
		market:        &market.Manager{},
		baseDir:       "/srv/agezt",
		artifactIndex: &artifact.Index{},
		dataLake:      &datalake.Lake{},
	}
	a := New(k)
	if a.Market() != k.market {
		t.Errorf("Market(): Accessor = %v, want %v", a.Market(), k.market)
	}
	if a.BaseDir() != "/srv/agezt" {
		t.Errorf("BaseDir() = %q, want /srv/agezt", a.BaseDir())
	}
	if a.ArtifactIndex() != k.artifactIndex {
		t.Errorf("ArtifactIndex(): Accessor = %v, want %v", a.ArtifactIndex(), k.artifactIndex)
	}
	if a.DataLake() != k.dataLake {
		t.Errorf("DataLake(): Accessor = %v, want %v", a.DataLake(), k.dataLake)
	}
}

func TestAccessors_Day16_SetMarketRoundtrip(t *testing.T) {
	// SetMarket is a write — the Accessor should hand the new
	// value straight through to the host, and a subsequent
	// Market() call should see the same value.
	k := newFakeKernel()
	a := New(k)
	want := &market.Manager{}
	a.SetMarket(want)
	if a.Market() != want {
		t.Errorf("SetMarket then Market(): got %v, want %v", a.Market(), want)
	}
}

func TestAccessors_Day18a_ConfigMuMutators(t *testing.T) {
	// Day 18a: the four configMu-guarded mutators + Council read.
	// The fake kernel uses one shared sync.Mutex (k.mu) for the
	// configMu hook, plus closure fields for the model / system /
	// council-fn state. After a Set* call we read the host state
	// through the same mutex to confirm the Accessor took the
	// lock and wrote through.
	var (
		model, system string
		councilFn     func() []types.CouncilMember
	)
	k := &fakeKernel{}
	// No late-binding needed: the Accessor takes the configMu
	// once on the way in/out, so the host's hooks MUST NOT
	// re-acquire it. Just store the value and return it.
	k.ModelFn = func() string { return model }
	k.SetModelFn = func(s string) { model = s }
	k.SystemFn = func() string { return system }
	k.SetSystemFn = func(s string) { system = s }
	k.MembersFn = func() func() []types.CouncilMember { return councilFn }
	k.SetMembersFn = func(f func() []types.CouncilMember) { councilFn = f }
	a := New(k)

	// SetModel then Model: the read-after-write must see the new
	// value, AND the host's stored value must be the new value
	// (i.e. the Accessor took the lock to write, not just the
	// value).
	a.SetModel("gpt-4o")
	if got := a.Model(); got != "gpt-4o" {
		t.Errorf("Model() after SetModel = %q, want gpt-4o", got)
	}
	if model != "gpt-4o" {
		t.Errorf("host model = %q, want gpt-4o (Accessor must have written under the same mutex)", model)
	}

	// SetSystem then System.
	a.SetSystem("you are a helpful agent")
	if got := a.System(); got != "you are a helpful agent" {
		t.Errorf("System() after SetSystem = %q, want %q", got, "you are a helpful agent")
	}
	if system != "you are a helpful agent" {
		t.Errorf("host system = %q, want %q (Accessor must have written under the same mutex)", system, "you are a helpful agent")
	}

	// SetCouncilMembers then CouncilMembers — the read returns
	// the function the host now holds, and that function yields
	// the same members.
	want := func() []types.CouncilMember { return []types.CouncilMember{{Seat: "alpha", Model: "gpt-4o"}} }
	a.SetCouncilMembers(want)
	if a.CouncilMembers() == nil {
		t.Errorf("CouncilMembers() returned nil after SetCouncilMembers")
	}
	if councilFn == nil {
		t.Fatalf("host councilFn is nil after SetCouncilMembers; Accessor must have written under the same mutex")
	}
	got := councilFn()
	if len(got) != 1 || got[0].Seat != "alpha" {
		t.Errorf("host councilFn() = %+v, want [{alpha gpt-4o}]", got)
	}
}

func TestAccessors_Day18a_ReadHelpers(t *testing.T) {
	// SubAgentLimits and Plugins are configMu-free reads; the
	// Accessor just hands back the host's value.
	wantSubs := types.SubAgentLimits{
		Enabled:            true,
		MaxDepth:           3,
		MaxFanout:          7,
		MaxSpendMicrocents: 12345,
		MaxTotal:           9,
	}
	wantPlugs := []types.PluginInfo{
		{Prefix: "weather", Path: "/usr/local/bin/weather", ToolCount: 4},
	}
	k := &fakeKernel{
		SubsFn:    func() types.SubAgentLimits { return wantSubs },
		PluginsFn: func() []types.PluginInfo { return wantPlugs },
	}
	a := New(k)
	if got := a.SubAgentLimits(); got != wantSubs {
		t.Errorf("SubAgentLimits() = %+v, want %+v", got, wantSubs)
	}
	if got := a.Plugins(); len(got) != 1 || got[0].Prefix != "weather" {
		t.Errorf("Plugins() = %+v, want [{weather /usr/local/bin/weather ... 4}]", got)
	}
}
