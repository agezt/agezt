// SPDX-License-Identifier: MIT

// Package binding hands the daemon's own kernel to the family that owns the
// ports.
//
// The daemon IS the daemon's own kernel. A family fills its OWN ports from
// it, and the adapter that holds the daemon never names the runtime — Go does
// not need the name to hand a value over. This bridge is that hand-over, and
// it lives in the application layer because the application owns the
// families' ports.
//
// The bridge has ONE face: it fills EVERY family's ports at once, for ONE
// scope of the daemon (the primary, or one tenant). It is what the
// COMPOSITION ROOT builds: the root is the one place that holds the daemon,
// and what it hands to the adapter is a scope — the families' capabilities,
// already filled — and never the daemon itself.
//
// A scope is a bundle of ports, not a daemon: an adapter that holds one can
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

	// Directory is the scope's own directory: the primary daemon's, or the
	// tenant's. The families that keep state on disk read it, and it is what
	// tells one scope from another.
	//
	// It is not named after the kernel's own verb: the kernel's BaseDir is the
	// DAEMON's directory, and a scope that carried the verb's name would name
	// a door it does not have.
	Directory string

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
		return RefusingScope(baseDir)
	}
	return Scope{
		ApprovalPorts:       appapprovals.PortsFromKernel(k),
		ArtifactPorts:       appartifacts.PortsFromKernel(k),
		BasedirPorts:        appbasedir.PortsFromKernel(k),
		BusPorts:            appeventbus.PortsFromKernel(k),
		CatalogPorts:        appcatalog.PortsFromKernel(k),
		ConfigCenterPorts:   appconfigcenter.PortsFromKernel(k),
		CorrelationPorts:    appcorrelation.PortsFromKernel(k),
		CouncilPorts:        appcouncil.PortsFromKernel(k, baseDir),
		CouncilEngines:      appcouncil.EnginesFromKernel(k),
		DataPorts:           appdata.PortsFromKernel(k),
		EdictPorts:          appedict.PortsFromKernel(k),
		ExecProfilePorts:    appexecprofile.PortsFromKernel(k),
		FilesRunner:         appfiles.RunnerFromKernel(k),
		JournalPorts:        appjournal.PortsFromKernel(k),
		LifecyclePorts:      applifecycle.PortsFromKernel(k),
		MarketPorts:         appmarket.PortsFromKernel(k),
		MemoryPorts:         appmemory.PortsFromKernel(k),
		OkrPorts:            appokr.PortsFromKernel(k),
		PersonaPorts:        apppersona.PortsFromKernel(k),
		PlanRunnerPorts:     appruns.PlanRunnerPortsFromKernel(k),
		PluginPorts:         appplugins.PortsFromKernel(k),
		ProviderPorts:       appprovider.PortsFromKernel(k),
		ReaperScanPorts:     appreaper.PortsFromKernel(k),
		ReflectionPorts:     appreflection.PortsFromKernel(k),
		ResearchEngines:     appresearch.EnginesFromKernel(k, appcorrelation.PortsFromKernel(k).Mint),
		RosterProfilePorts:  approster.ProfilePortsFromKernel(k),
		RunPorts:            appruns.RunPortsFromKernel(k),
		RunsPorts:           appruns.PortsFromKernel(k),
		SchedulePorts:       appschedule.PortsFromKernel(k),
		SeatsPorts:          appseats.PortsFromKernel(k),
		SkillPorts:          appskill.PortsFromKernel(k),
		StandingPorts:       appstanding.PortsFromKernel(k),
		StatePorts:          appstate.PortsFromKernel(k),
		SteerRuns:           appsteer.RunsFromKernel(k),
		SystemPorts:         appsystem.PortsFromKernel(k),
		TastePorts:          apptaste.PortsFromKernel(k),
		ToolsPorts:          apptools.PortsFromKernel(k),
		WorkboardPorts:      appworkboard.PortsFromKernel(k),
		WorkflowEnginePorts: appworkflow.PortsFromKernel(k),
		WorldPorts:          appworld.PortsFromKernel(k),
		Directory:           baseDir,
		filled:              true,
	}
}

// RefusingScope is the scope that refuses every family.
//
// It is what a scope looks like when there is no daemon behind it: the
// families that take a handle refuse, the others are empty. An adapter that
// was built without a scope asks for this one rather than calling into
// nothing.
//
// The scope keeps its DIRECTORY: a scope that refuses still knows where it
// is, and the adapter's own directory comes from it.
func RefusingScope(baseDir string) Scope {
	return Scope{
		CouncilPorts:    appcouncil.PortsFromKernel(nil, baseDir),
		FilesRunner:     appfiles.RunnerFromKernel(nil),
		PlanRunnerPorts: appruns.PlanRunnerPortsFromKernel(nil),
		ResearchEngines: appresearch.EnginesFromKernel(nil, func() string { return "" }),
		SteerRuns:       appsteer.RunsFromKernel(nil),
		Directory:       baseDir,
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
		return RefusingScope(baseDir), nil
	}
	t, err := reg.Acquire(id, now)
	if err != nil {
		return RefusingScope(baseDir), err
	}
	k, ok := t.Kernel.(*runtime.Kernel)
	if !ok {
		// The tenant's own directory is only known once the tenant is open, so
		// a tenant that is not the daemon's own refuses in the caller's
		// directory — the refusal names no tenant's state.
		return RefusingScope(baseDir), nil
	}
	return ScopeFromKernel(k, t.BaseDir), nil
}
