// SPDX-License-Identifier: MIT

// Package binding hands the daemon's handle to the family that owns the
// ports.
//
// The handle is the daemon's own kernel. A family fills its OWN ports from
// it, and the adapter that holds the handle never names the runtime — Go does
// not need the name to hand a value over. This bridge is that hand-over, and
// it lives in the application layer because the application owns the
// families' ports.
//
// The bridge has TWO faces:
//
//   - Fill fills ONE family's ports from the handle. The family owns the fill;
//     the bridge only hands the handle to the right family.
//   - Scope fills EVERY family's ports at once, for ONE scope of the daemon
//     (the primary, or one tenant). It is what the COMPOSITION ROOT builds:
//     the root is the one place that holds the handle, and what it hands to
//     the adapter is a scope — the families' capabilities, already filled —
//     and never the handle itself.
//
// A scope is a bundle of ports, not a handle: an adapter that holds one can
// reach every family's capabilities and can reach NOTHING of the kernel
// behind them.
package binding

import (
	"time"

	appapprovals "github.com/agezt/agezt/kernel/app/approvals"
	appartifacts "github.com/agezt/agezt/kernel/app/artifacts"
	appbasedir "github.com/agezt/agezt/kernel/app/basedir"
	appcatalog "github.com/agezt/agezt/kernel/app/catalog"
	appconfigcenter "github.com/agezt/agezt/kernel/app/configcenter"
	appcorrelation "github.com/agezt/agezt/kernel/app/correlation"
	appcouncil "github.com/agezt/agezt/kernel/app/council"
	appdata "github.com/agezt/agezt/kernel/app/data"
	appedict "github.com/agezt/agezt/kernel/app/edict"
	appeventbus "github.com/agezt/agezt/kernel/app/eventbus"
	appexecprofile "github.com/agezt/agezt/kernel/app/execprofile"
	appfiles "github.com/agezt/agezt/kernel/app/files"
	appjournal "github.com/agezt/agezt/kernel/app/journal"
	applifecycle "github.com/agezt/agezt/kernel/app/lifecycle"
	appmarket "github.com/agezt/agezt/kernel/app/market"
	appmemory "github.com/agezt/agezt/kernel/app/memory"
	appokr "github.com/agezt/agezt/kernel/app/okr"
	apppersona "github.com/agezt/agezt/kernel/app/persona"
	appplugins "github.com/agezt/agezt/kernel/app/plugins"
	appprovider "github.com/agezt/agezt/kernel/app/provider"
	appreaper "github.com/agezt/agezt/kernel/app/reaper"
	appreflection "github.com/agezt/agezt/kernel/app/reflection"
	appresearch "github.com/agezt/agezt/kernel/app/research"
	approster "github.com/agezt/agezt/kernel/app/roster"
	appruns "github.com/agezt/agezt/kernel/app/runs"
	appschedule "github.com/agezt/agezt/kernel/app/schedule"
	appseats "github.com/agezt/agezt/kernel/app/seats"
	appskill "github.com/agezt/agezt/kernel/app/skill"
	appstanding "github.com/agezt/agezt/kernel/app/standing"
	appstate "github.com/agezt/agezt/kernel/app/state"
	appsteer "github.com/agezt/agezt/kernel/app/steer"
	appsystem "github.com/agezt/agezt/kernel/app/system"
	apptaste "github.com/agezt/agezt/kernel/app/taste"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	appworkboard "github.com/agezt/agezt/kernel/app/workboard"
	appworkflow "github.com/agezt/agezt/kernel/app/workflow"
	appworld "github.com/agezt/agezt/kernel/app/world"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
)

// Fill fills a family's ports from the daemon's handle.
//
// The family that owns the ports fills them; this bridge only hands the
// handle to the right family. A handle that is not the daemon's own yields
// the family's EMPTY ports, and an empty port refuses rather than calling
// into nothing.
func Fill[T any](handle any, fill func(*runtime.Kernel) T) T {
	k, ok := handle.(*runtime.Kernel)
	if !ok {
		var zero T
		return zero
	}
	return fill(k)
}

// Scope holds the ports of EVERY family the adapter reaches, for ONE scope of
// the daemon: the primary's, or one tenant's.
//
// The scope is the composition root's gift to the adapter. The root holds the
// handle; the root fills the scope; the adapter reads the scope. The adapter
// can reach every family's capabilities through it and can reach NOTHING of
// the kernel behind them — there is no handle left to walk.
//
// A scope that was never filled is EMPTY: every family's ports are the zero
// value, and the families that refuse a foreign handle refuse here too.
type Scope struct {
	ApprovalPorts       appapprovals.Ports
	ArtifactPorts       appartifacts.Ports
	BasedirPorts        appbasedir.Ports
	BusPorts            appeventbus.Ports
	CatalogPorts        appcatalog.Ports
	ConfigCenterPorts   appconfigcenter.Ports
	CorrelationPorts    appcorrelation.Ports
	CouncilPorts        appcouncil.Ports
	CouncilEngines      appcouncil.Engines
	DataPorts           appdata.Ports
	EdictPorts          appedict.Ports
	ExecProfilePorts    appexecprofile.Ports
	FilesRunner         appfiles.Runner
	JournalPorts        appjournal.Ports
	LifecyclePorts      applifecycle.Ports
	MarketPorts         appmarket.Ports
	MemoryPorts         appmemory.Ports
	OkrPorts            appokr.Ports
	PersonaPorts        apppersona.Ports
	PlanRunnerPorts     appruns.PlanRunnerPorts
	PluginPorts         appplugins.Ports
	ProviderPorts       appprovider.Ports
	ReaperScanPorts     appreaper.Ports
	ReflectionPorts     appreflection.Ports
	ResearchEngines     appresearch.Engines
	RosterProfilePorts  approster.ProfilePorts
	RunPorts            appruns.RunPorts
	RunsPorts           appruns.Ports
	SchedulePorts       appschedule.Ports
	SeatsPorts          appseats.Ports
	SkillPorts          appskill.Ports
	StandingPorts       appstanding.Ports
	StatePorts          appstate.Ports
	SteerRuns           appsteer.Runs
	SystemPorts         appsystem.Ports
	TastePorts          apptaste.Ports
	ToolsPorts          apptools.Ports
	WorkboardPorts      appworkboard.Ports
	WorkflowEnginePorts appworkflow.Ports
	WorldPorts          appworld.Ports

	// BaseDir is the scope's own directory: the primary daemon's, or the
	// tenant's. The families that keep state on disk read it, and it is what
	// tells one scope from another.
	BaseDir string

	// filled records that this scope was filled from the daemon's own kernel.
	// A scope that was not filled is refused, and an adapter that was built
	// without a scope at all must not call into nothing.
	filled bool
}

// Filled reports whether the scope was filled from the daemon's own kernel.
func (s Scope) Filled() bool { return s.filled }

// ScopeFromKernel fills every family's ports from the daemon's own kernel.
//
// baseDir is the scope's own directory: the families that keep state on disk
// (the council's settings store) read it, so a tenant's scope is filled with
// the TENANT's directory.
func ScopeFromKernel(k *runtime.Kernel, baseDir string) Scope {
	if k == nil {
		return RefusingScope()
	}
	return Scope{
		ApprovalPorts:       Fill(k, appapprovals.PortsFromKernel),
		ArtifactPorts:       Fill(k, appartifacts.PortsFromKernel),
		BasedirPorts:        Fill(k, appbasedir.PortsFromKernel),
		BusPorts:            Fill(k, appeventbus.PortsFromKernel),
		CatalogPorts:        Fill(k, appcatalog.PortsFromKernel),
		ConfigCenterPorts:   Fill(k, appconfigcenter.PortsFromKernel),
		CorrelationPorts:    Fill(k, appcorrelation.PortsFromKernel),
		CouncilPorts:        appcouncil.PortsFromKernel(k, baseDir),
		CouncilEngines:      appcouncil.EnginesFromKernel(k),
		DataPorts:           Fill(k, appdata.PortsFromKernel),
		EdictPorts:          Fill(k, appedict.PortsFromKernel),
		ExecProfilePorts:    Fill(k, appexecprofile.PortsFromKernel),
		FilesRunner:         appfiles.RunnerFromHandle(k),
		JournalPorts:        Fill(k, appjournal.PortsFromKernel),
		LifecyclePorts:      Fill(k, applifecycle.PortsFromKernel),
		MarketPorts:         Fill(k, appmarket.PortsFromKernel),
		MemoryPorts:         Fill(k, appmemory.PortsFromKernel),
		OkrPorts:            Fill(k, appokr.PortsFromKernel),
		PersonaPorts:        Fill(k, apppersona.PortsFromKernel),
		PlanRunnerPorts:     appruns.PlanRunnerPortsFromHandle(k),
		PluginPorts:         Fill(k, appplugins.PortsFromKernel),
		ProviderPorts:       Fill(k, appprovider.PortsFromKernel),
		ReaperScanPorts:     Fill(k, appreaper.PortsFromKernel),
		ReflectionPorts:     Fill(k, appreflection.PortsFromKernel),
		ResearchEngines:     appresearch.EnginesFromHandle(k, appcorrelation.PortsFromKernel(k).Mint),
		RosterProfilePorts:  Fill(k, approster.ProfilePortsFromKernel),
		RunPorts:            Fill(k, appruns.RunPortsFromKernel),
		RunsPorts:           Fill(k, appruns.PortsFromKernel),
		SchedulePorts:       Fill(k, appschedule.PortsFromKernel),
		SeatsPorts:          Fill(k, appseats.PortsFromKernel),
		SkillPorts:          Fill(k, appskill.PortsFromKernel),
		StandingPorts:       Fill(k, appstanding.PortsFromKernel),
		StatePorts:          Fill(k, appstate.PortsFromKernel),
		SteerRuns:           appsteer.RunsFromHandle(k),
		SystemPorts:         Fill(k, appsystem.PortsFromKernel),
		TastePorts:          Fill(k, apptaste.PortsFromKernel),
		ToolsPorts:          Fill(k, apptools.PortsFromKernel),
		WorkboardPorts:      Fill(k, appworkboard.PortsFromKernel),
		WorkflowEnginePorts: Fill(k, appworkflow.PortsFromKernel),
		WorldPorts:          Fill(k, appworld.PortsFromKernel),
		BaseDir:             baseDir,
		filled:              true,
	}
}

// ScopeFromHandle fills the scope from a handle that is not named by the
// caller.
//
// A handle that is not the daemon's own yields the REFUSING scope: the
// families that take a handle refuse, the families that take a kernel yield
// empty ports, and nothing is called into that does not exist.
func ScopeFromHandle(handle any, baseDir string) Scope {
	if k, ok := handle.(*runtime.Kernel); ok {
		return ScopeFromKernel(k, baseDir)
	}
	return RefusingScope()
}

// Resolve is the scope for a value the ADAPTER was handed.
//
// A scope stays the scope it is; a handle is filled into one; anything else
// refuses. The adapter never stores what it was handed either way: it stores
// the scope, and the scope is all it can reach.
func Resolve(value any, baseDir string) Scope {
	if scope, ok := value.(Scope); ok {
		return scope
	}
	return ScopeFromHandle(value, baseDir)
}

// RefusingScope is the scope that refuses every family.
//
// It is what a scope looks like when there is no daemon behind it: the
// families that take a handle refuse, the others are empty. An adapter that
// was built without a scope asks for this one rather than calling into
// nothing.
func RefusingScope() Scope {
	return Scope{
		CouncilPorts:    appcouncil.PortsFromHandle(nil, ""),
		FilesRunner:     appfiles.RunnerFromHandle(nil),
		PlanRunnerPorts: appruns.PlanRunnerPortsFromHandle(nil),
		ResearchEngines: appresearch.EnginesFromHandle(nil, func() string { return "" }),
		SteerRuns:       appsteer.RunsFromHandle(nil),
	}
}

// TenantAcquirer opens one tenant's kernel.
//
// It is the tenant registry's own half: the adapter holds the registry for
// the AUTHORIZATION it does, and the SCOPE of a tenant is filled here, in the
// application layer, so that no handle crosses the boundary.
type TenantAcquirer interface {
	Acquire(id string, now time.Time) (*tenant.Tenant, error)
}

// ScopeForTenant fills the scope of ONE tenant.
//
// The tenant's kernel is opened here, in the application layer, and what
// leaves this function is the tenant's scope — its families' capabilities —
// and never the tenant's kernel. A tenant whose kernel is not the daemon's
// own yields the refusing scope: the tenant is isolated, and its refusal
// does not leak into the primary's.
func ScopeForTenant(reg TenantAcquirer, id string, now time.Time, baseDir string) (Scope, error) {
	if reg == nil {
		return RefusingScope(), nil
	}
	t, err := reg.Acquire(id, now)
	if err != nil {
		return RefusingScope(), err
	}
	k, ok := t.Kernel.(*runtime.Kernel)
	if !ok {
		return RefusingScope(), nil
	}
	return ScopeFromKernel(k, t.BaseDir), nil
}
