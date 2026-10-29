// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"

	"github.com/agezt/agezt/kernel/app/configtool"
	"github.com/agezt/agezt/kernel/app/introspect"
	"github.com/agezt/agezt/kernel/app/overseer"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/boardapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/configapi"
	"github.com/agezt/agezt/kernel/contract/datalakeapi"
	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/contract/journalapi"
	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/contract/reasoningapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	"github.com/agezt/agezt/kernel/contract/toolregapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/contract/workflowapi"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/modules/board/board"
	"github.com/agezt/agezt/kernel/runtime"
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
func toolKernelDeps(k *runtime.Kernel, baseDir string, stdout io.Writer) toolregapi.KernelDeps {
	return toolregapi.KernelDeps{
		// The tool_forge tool drives the forge's lifecycle through the
		// contract's port (W3.39g).
		ForgeRunner: forgeRunnerPort(k),
		// The code_exec tool installs itself as the Verifier's executor
		// through a plain closure (W3.39g): a function value names no type,
		// so the row stays in the contract without naming the kernel.
		ConductorExec:   conductorExecPort(k),
		Bus:             busPort(k.Bus()),
		Artifacts:       k.ArtifactIndex(),
		Lake:            lakeStore(k),
		Journal:         journalPort(k.Journal()),
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
		Overseer: overseer.NewKernelSource(overseer.PortsFromKernel(k), baseDir),
		// The introspect tool reads the daemon's own live state through a
		// contract port; the app-layer adapter gathers it from the live
		// kernel, because the tool is a plugin and may reach neither.
		Introspect: introspect.NewKernelSource(introspect.PortsFromKernel(k)),
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
//
// The rule is written ONCE: the ports say "no daemon" themselves, and the
// composition root does not spell it again.
func configSource(k *runtime.Kernel) configapi.Kernel {
	return configtool.NewKernelSource(configtool.PortsFromKernel(k))
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
// port. A missing lake must stay a NIL port, not a port wrapping a nil
// implementation — the check is on the concrete type on purpose, because a nil
// *Lake handed to an interface parameter is already a non-nil interface.
//
// The kernel holds the port itself now (W3.42): the daemon opened the lake
// and injected the contract's port, so there is nothing left to wrap — and
// nothing left that could turn a missing lake into a non-nil port.
func lakeStore(k *runtime.Kernel) datalakeapi.Lake {
	if k == nil {
		return nil
	}
	return k.DataLake()
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
// contract's port.
//
// The kernel holds the port itself now (W3.45): the daemon opened the store
// and injected the contract's port, so there is nothing left to wrap — and
// nothing left that could turn a missing store into a non-nil port.
func forgeStore(k *runtime.Kernel) toolforgeapi.Store {
	if k == nil {
		return nil
	}
	return k.ToolForge()
}

// forgeRunnerPort turns the live kernel into the contract's forge runner.
//
// The kernel's own forge methods already speak the contract's rows (the
// script tool and the approval verdict are aliases of the contract's), so
// the kernel satisfies the port DIRECTLY — there is no adapter in between,
// and the wire is the same row all the way down.
//
// A nil kernel must stay a NIL PORT: the tool_forge tool then reports that
// the forge is unavailable instead of calling into nothing.
func forgeRunnerPort(k *runtime.Kernel) toolforgeapi.Runner {
	if k == nil {
		return nil
	}
	return k
}

// conductorExecPort returns the closure that installs the Verifier's code
// executor.
//
// It is a CLOSURE on purpose: a function value names no type, so the row
// stays in the contract without the contract naming the kernel. The kernel's
// own setter takes the contract's runner (the runtime's CodeExecutor is an
// alias of it), so the value is handed over without a translation.
func conductorExecPort(k *runtime.Kernel) func(x toolregapi.ScriptRunner) {
	return func(x toolregapi.ScriptRunner) {
		if k == nil {
			return
		}
		k.SetConductorExec(x)
	}
}

// artifactBytes returns the closure that resolves an artifact reference to
// its bytes.
//
// It is a CLOSURE on purpose: a function value names no type, so the row
// stays in the contract without the contract naming the artifact module. The
// blob store re-verifies the hash on the way out, so a tampered blob is
// refused rather than attached to a message.
func artifactBytes(k *runtime.Kernel) func(ref string) ([]byte, error) {
	return func(ref string) ([]byte, error) {
		if k == nil {
			return nil, fmt.Errorf("artifact store unavailable")
		}
		a := k.Artifacts()
		if a == nil {
			return nil, fmt.Errorf("artifact store unavailable")
		}
		return a.Get(ref)
	}
}

// runtimeConfigPort answers the contract's pre-Open port over the daemon's
// runtime configuration (W3.39h).
//
// It lives HERE, at the composition root, because that is the one place that
// knows both the contract and the daemon. The adapter used to live inside the
// registry — the registry was the one place that knew both — but the registry
// now lives in the CONTRACT layer (kernel/contract/toolregapi) and may not
// name the daemon's runtime at all. Moving the adapter down to the daemon is
// what made the move possible, and it is also where it belongs: the daemon
// owns the configuration.
//
// The contract's runner and the daemon's runner interface have the same
// method set, so the value is assigned directly — the port translates
// nothing.
type runtimeConfigPort struct{ cfg *runtime.Config }

// SetScriptRunner installs the runner that executes forged script tools
// (M794) in the daemon's configuration.
//
// A missing configuration is refused rather than dereferenced: the port would
// otherwise install a runner into nothing and report success for a
// configuration that does not exist.
func (p runtimeConfigPort) SetScriptRunner(r toolregapi.ScriptRunner) {
	if p.cfg == nil {
		return
	}
	p.cfg.ScriptRunner = r
}

// busPort turns the live bus into the contract's publish port (W3.39f).
//
// A nil bus must stay a NIL PORT: a port that wraps a nil implementation is
// not nil, so the tool would publish into nothing instead of skipping the
// event. The check is on the concrete type on purpose — a nil *bus.Bus handed
// to an interface parameter is already a non-nil interface.
func busPort(b *bus.Bus) eventapi.Publisher {
	if b == nil {
		return nil
	}
	return busPublisher{b: b}
}

// busPublisher publishes one event on the live bus.
//
// NOTHING is translated on the way in: the spec the contract's port takes is
// the same row the bus takes — the platform's event package keeps it as an
// alias of the contract's — so what the tool publishes is exactly what the
// bus writes. The port's shape is narrower on purpose (it answers "did it
// land?" and not "which event did it write?"), because the durable envelope
// it would return carries the hash chain, and the contract layer may not
// name that.
type busPublisher struct{ b *bus.Bus }

// Publish writes the event durably and fans it out to the live subscribers.
func (p busPublisher) Publish(spec eventapi.Spec) error {
	_, err := p.b.Publish(spec)
	return err
}

// journalPort turns the live journal into the contract's read port (W3.39f).
//
// The same nil-port rule as the bus's: a nil *journal.Journal handed to an
// interface parameter is a NON-NIL interface, so the runs tool would bind a
// reader that is not there instead of reporting that the run history is
// unavailable.
func journalPort(j *journal.Journal) journalapi.Reader {
	if j == nil {
		return nil
	}
	return journalReader{j: j}
}

// journalReader reads recent events from the live journal.
//
// It is the ADAPTER the durable envelope needs: the journal hands back the
// durable event (with its hash chain and its sequence), and the contract's
// port hands back the read PROJECTION — the kind, the correlation, the
// timestamp and the payload, which is everything a reader folds. The copy is
// field by field and it is measured: a reader that saw a different event
// than the journal holds would fold a different history.
type journalReader struct{ j *journal.Journal }

// Tail returns the last n events, oldest first.
func (r journalReader) Tail(n int) ([]journalapi.Record, error) {
	evs, err := r.j.Tail(n)
	if err != nil {
		return nil, err
	}
	out := make([]journalapi.Record, 0, len(evs))
	for _, e := range evs {
		if e == nil {
			continue
		}
		out = append(out, journalapi.Record{
			Kind:          e.Kind,
			CorrelationID: e.CorrelationID,
			TSUnixMS:      e.TSUnixMS,
			Payload:       e.Payload,
		})
	}
	return out, nil
}
