// SPDX-License-Identifier: MIT

package main

import (
	"io"

	"github.com/agezt/agezt/kernel/app/configtool"
	"github.com/agezt/agezt/kernel/app/introspect"
	"github.com/agezt/agezt/kernel/app/overseer"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/artifactapi"
	"github.com/agezt/agezt/kernel/contract/boardapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/configapi"
	"github.com/agezt/agezt/kernel/contract/datalakeapi"
	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/contract/reasoningapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/contract/workflowapi"
	"github.com/agezt/agezt/kernel/modules/artifacts/artifact"
	"github.com/agezt/agezt/kernel/modules/artifacts/datalake"
	"github.com/agezt/agezt/kernel/modules/board/board"
	"github.com/agezt/agezt/kernel/modules/extensions/toolforge"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/toolreg"
)

// toolKernelDeps assembles the post-Open dependencies the registry's
// Configure phase injects into the built tools.
//
// It is a named function (not an inline literal in runDaemon) because the
// wiring it performs is a CONTRACT with the tools: the daemon hands over the
// app-layer implementations of the ports those tools read through. A port
// that is silently dropped here would leave the tool answering "not
// available" on every live daemon — invisible to every test that builds the
// tool by hand. The composition root's own test therefore asserts that each
// port this function supplies is present and answers the live kernel.
func toolKernelDeps(k *runtime.Kernel, baseDir string, stdout io.Writer) toolreg.KernelDeps {
	return toolreg.KernelDeps{
		K:               k,
		Bus:             k.Bus(),
		Artifacts:       artifactPort(k.ArtifactIndex()),
		Lake:            lakeStore(k),
		Journal:         k.Journal(),
		BaseDir:         baseDir,
		Stdout:          stdout,
		NetguardPublish: netguardPublish(k.Bus()),
		// The council, conductor and research tools drive the daemon's
		// reasoning surface through the contract's port (W3.39b).
		Reasoning: reasoningPort(k),
		// The workflow tool drives the daemon's workflow library through the
		// contract's write-and-run port (W3.39c).
		WorkflowRunner: workflowPort(k),
		// The workboard tool drives the daemon's workboard through the
		// contract's move-and-claim port (W3.39c).
		WorkboardRunner: workboardPort(k),
		// The MCP tool drives the daemon's MCP registry through the
		// contract's port (W3.39d).
		MCP: mcpPort(k),
		// The standing-orders tool drives the daemon's standing orders
		// through the contract's port (W3.39d), and checks an agent profile
		// through a plain closure.
		Standing:    standingPort(k),
		AgentLookup: agentLookup(k),
		// The schedule tool writes its entries through the contract's
		// cadence port (W3.39e).
		Schedules: schedulesPort(k),
		// The skill tool authors and retires its own procedures through the
		// contract's forge port (W3.39e).
		SkillForge:   skillForgePort(k),
		SkillBundles: skillBundlesPort(k),
		// The overseer tool steers the fleet through a contract port; the
		// implementation that answers it needs the live kernel and the board
		// store, so the daemon builds it here (the tool is a plugin and may
		// reach neither).
		Overseer: overseer.NewKernelSource(k, baseDir),
		// The introspect tool reads the daemon's own live state through a
		// contract port; the app-layer adapter gathers it from the live
		// kernel, because the tool is a plugin and may reach neither.
		Introspect: introspect.NewKernelSource(k),
		// The tool_forge tool reads the script-tool store through a contract
		// port; the daemon hands the live store over, because the tool is a
		// plugin and may not name the module that keeps it. A nil store must
		// stay a nil port, or the tool would call into a nil implementation
		// instead of reporting that the store is unavailable.
		Forge: forgeStore(k),
		// The workflow tool reads the workflow library through a contract
		// port; the daemon hands the live store over, for the same reason.
		Workflows: workflowStore(k),
		// The workboard tool reads the task store through a contract port;
		// the daemon hands the live store over, for the same reason.
		Workboard: workboardStore(k),
		// The config tool writes through a contract port — the agent-scoped
		// override path and the live-apply reload. The implementation that
		// answers it needs the live kernel, so the daemon builds it here (the
		// tool is a plugin and may reach neither).
		Config: configSource(k),
	}
}

// configSource hands the live daemon's port to the tool registry — the
// config tool's way into the roster and the provider. The nil-port rule is
// the same as the forge store's: a daemon with no kernel hands over a nil
// port, not a port wrapping nothing.
func configSource(k *runtime.Kernel) configapi.Kernel {
	if k == nil {
		return nil
	}
	return configtool.NewKernelSource(k)
}

// workboardStore hands the live task store to the tool registry as the
// contract's port — the same nil-port rule as the forge store.
func workboardStore(k *runtime.Kernel) workboardapi.Store {
	if k == nil {
		return nil
	}
	s := k.Workboard()
	if s == nil {
		return nil
	}
	return s
}

// lakeStore hands the live data lake to the tool registry as the contract's
// port. A nil lake must stay a NIL port, not a port wrapping a nil
// implementation — the check is on the concrete type on purpose, because a nil
// *Lake handed to an interface parameter is already a non-nil interface.
func lakeStore(k *runtime.Kernel) datalakeapi.Lake {
	if k == nil {
		return nil
	}
	return lakePort(k.DataLake())
}

// lakePort turns the concrete lake into the contract's port.
//
// A nil lake must stay a NIL PORT: a port that wraps a nil implementation is
// not nil, so the tool would call into nothing instead of reporting that the
// lake is unavailable. The check is on the concrete type on purpose — a nil
// *Lake handed to an interface parameter is already a non-nil interface.
func lakePort(l *datalake.Lake) datalakeapi.Lake {
	if l == nil {
		return nil
	}
	return l
}

// workflowStore hands the live workflow library to the tool registry as the
// contract's port — the same nil-port rule as the forge store.
func workflowStore(k *runtime.Kernel) workflowapi.Store {
	if k == nil {
		return nil
	}
	s := k.Workflows()
	if s == nil {
		return nil
	}
	return s
}

// artifactPort turns the concrete artifact index into the contract's port.
//
// A nil index must stay a NIL PORT: a port that wraps a nil implementation is
// not nil, so the tool would call into nothing instead of reporting that the
// index is unavailable. The check is on the concrete type on purpose — a nil
// *artifact.Index handed to an interface parameter is already a non-nil
// interface, and the tools would bind it and crash on the first read.
func artifactPort(idx *artifact.Index) artifactapi.Index {
	if idx == nil {
		return nil
	}
	return idx
}

// boardPort turns the concrete board store into the contract's port.
//
// The same nil-port rule as the lake's: the board store is opened late and
// the daemon leaves it nil when it could not be opened — a nil *board.Store
// handed to an interface parameter is a NON-NIL interface, so the board tool
// would bind a store that is not there and panic on the first post instead
// of reporting that the board is unavailable.
func boardPort(st *board.Store) boardapi.Store {
	if st == nil {
		return nil
	}
	return st
}

// reasoningPort hands the kernel's reasoning surface to the tool registry as
// the contract's port (W3.39b) — the council, conductor and research tools
// drive it.
//
// The same nil-port rule as the other ports: a nil *runtime.Kernel handed to
// an interface parameter is a NON-NIL interface, and the three tools would
// bind a runner that is not there instead of reporting that reasoning is
// unavailable.
func reasoningPort(k *runtime.Kernel) reasoningapi.Runner {
	if k == nil {
		return nil
	}
	return k
}

// workflowPort hands the kernel's workflow library to the tool registry as
// the contract's write-and-run port (W3.39c) — the workflow tool authors and
// runs durable workflows through it.
//
// The same nil-port rule: a nil kernel handed to an interface parameter is a
// non-nil interface, and the tool would bind a runner that is not there.
func workflowPort(k *runtime.Kernel) workflowapi.Runner {
	if k == nil {
		return nil
	}
	return k
}

// workboardPort hands the kernel's workboard to the tool registry as the
// contract's move-and-claim port (W3.39c) — the workboard tool drives the
// durable, journaled path for typed work items through it.
//
// The same nil-port rule: a nil kernel handed to an interface parameter is a
// non-nil interface, and the tool would bind a runner that is not there.
func workboardPort(k *runtime.Kernel) workboardapi.Runner {
	if k == nil {
		return nil
	}
	return k
}

// mcpPort hands the kernel's MCP registry to the tool registry as the
// contract's port (W3.39d) — the MCP tool adds, attaches, detaches and
// removes servers through it.
//
// The same nil-port rule: a nil kernel handed to an interface parameter is a
// non-nil interface, and the tool would bind a runner that is not there.
func mcpPort(k *runtime.Kernel) mcpapi.Runner {
	if k == nil {
		return nil
	}
	return k
}

// standingPort hands the kernel's standing orders to the tool registry as
// the contract's port (W3.39d) — the standing-orders tool journals, removes
// and lists them through it.
//
// The same nil-port rule: a nil kernel handed to an interface parameter is a
// non-nil interface, and the tool would bind a host that is not there.
func standingPort(k *runtime.Kernel) standingapi.Host {
	if k == nil {
		return nil
	}
	return k
}

// agentLookup hands the daemon's roster lookup to the tool registry as a
// plain closure (W3.39d) — the standing-orders tool checks an agent profile
// through it.
//
// A closure on purpose: the roster is a module, and a row a plugin names
// cannot live in a module's implementation package — but a function value
// names no type at all. And a daemon with no roster must hand over NOTHING:
// a closure over a nil roster would panic on the first lookup, where a nil
// closure leaves the tool without the lookup it can do without.
func agentLookup(k *runtime.Kernel) func(ref string) (agentapi.Profile, bool) {
	if k == nil {
		return nil
	}
	r := k.Roster()
	if r == nil {
		return nil
	}
	return r.Get
}

// schedulesPort hands the daemon's cadence store to the tool registry as the
// contract's port (W3.39e) — the schedule tool writes its entries through
// it.
//
// The same nil-port rule: a nil store handed to an interface parameter is a
// non-nil interface, and the tool would bind a store that is not there.
func schedulesPort(k *runtime.Kernel) cadenceapi.Store {
	if k == nil {
		return nil
	}
	s := k.Schedules()
	if s == nil {
		return nil
	}
	return s
}

// skillForgePort hands the daemon's skills forge to the tool registry as the
// contract's port (W3.39e) — the skill tool authors, promotes and retires
// its own procedures through it.
//
// The same nil-port rule: a nil forge handed to an interface parameter is a
// non-nil interface, and the tool would bind a forge that is not there.
func skillForgePort(k *runtime.Kernel) skillapi.Forge {
	if k == nil {
		return nil
	}
	f := k.Forge()
	if f == nil {
		return nil
	}
	return f
}

// skillBundlesPort hands the forge's bundle store to the tool registry as
// the contract's port (W3.39e). A nil store must stay a nil port, or the
// tool would call into a nil implementation instead of reporting that
// bundles are unavailable.
func skillBundlesPort(k *runtime.Kernel) skillapi.BundleStore {
	if k == nil {
		return nil
	}
	f := k.Forge()
	if f == nil {
		return nil
	}
	b := f.Bundles()
	if b == nil {
		return nil
	}
	return b
}

// forgeStore hands the live script-tool store to the tool registry as the
// contract's port. It is a named function so the nil-port rule is stated once
// and can be tested on its own.
func forgeStore(k *runtime.Kernel) toolforgeapi.Store {
	if k == nil {
		return nil
	}
	return forgePort(k.ToolForge())
}

// forgePort turns the concrete store into the contract's port.
//
// A nil store must stay a NIL PORT: a port that wraps a nil implementation is
// not nil, so the tool would call into nothing instead of reporting that the
// store is unavailable. The check is on the concrete type on purpose — a
// nil *Store handed to an interface parameter is already a non-nil interface.
func forgePort(s *toolforge.Store) toolforgeapi.Store {
	if s == nil {
		return nil
	}
	return s
}
