// SPDX-License-Identifier: MIT

// Package toolreg is the first-party tool registry (Phase 2.2). Each built-in
// tool contributes a Spec — how to build it from boot-time deps, plus optional
// lifecycle hooks for the phases the daemon used to hand-wire with string-keyed
// downcasts in cmd/agezt/main.go:
//
//   - Build       pre-kernel construction (env-configured, may skip itself)
//   - PreOpen     mutate the runtime Config before runtime.Open
//   - Configure   post-Open injection (kernel, bus, artifact index, …)
//   - Late        wiring that needs live channels / the board
//
// Specs register into a package-level registry (mirroring builtinchannels);
// the daemon calls BuildAll once and drives the resulting Set through the
// phases. Netguard-capable tools implement NetguardAware and are wired
// generically by Configure — no hand-listed type switch — and NetguardGaps
// lets a test enumerate, FROM the registry, any Netguard spec whose built
// instances cannot receive the audit callback.
package toolreg

import (
	"context"
	"io"
	"sync"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/contract/acpapi"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/artifactapi"
	"github.com/agezt/agezt/kernel/contract/boardapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/configapi"
	"github.com/agezt/agezt/kernel/contract/datalakeapi"
	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/contract/introspectapi"
	"github.com/agezt/agezt/kernel/contract/journalapi"
	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/contract/overseerapi"
	"github.com/agezt/agezt/kernel/contract/pluginapi"
	"github.com/agezt/agezt/kernel/contract/reasoningapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	"github.com/agezt/agezt/kernel/contract/toolregapi"
	"github.com/agezt/agezt/kernel/contract/wardenapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/contract/workflowapi"
	"github.com/agezt/agezt/kernel/runtime"
)

// BuildDeps carries everything a tool may need at pre-kernel construction
// time. Env is read through Get (injectable for tests); AllowAll is the master
// permissive switch (AGEZT_ALLOW_ALL=1) the daemon resolves once.
type BuildDeps struct {
	BaseDir       string
	WorkspaceRoot string
	// Warden is the isolation engine the shell and code_exec tools route
	// their process launches through. It is the CONTRACT's port: the tools
	// are plugins, and a plugin may not name the platform package that keeps
	// the engine (W3.39f).
	Warden        wardenapi.Engine
	Stderr        io.Writer
	Get           func(name string) string
	AllowAll      bool
	NotifyTargets map[string][]string
	// ACP is the ACP-agent catalogue port: the acp_agent tool asks it which
	// ACP agents this host has installed and which launch command a chosen
	// slug maps to. The tool is a plugin, so it may not name the catalogue
	// module; the daemon supplies the contract's port
	// (kernel/contract/acpapi). Nil when unwired — the tool then registers
	// only when a default command is configured.
	ACP acpapi.Catalog
	// Plugins is the out-of-process plugin spawn port: the built-in tools
	// plugin hosts child processes, and the host side of that (the binary
	// pinning, the frame caps, the callback bridge, the teardown) lives in
	// a module the plugin may not name. The daemon supplies the contract's
	// port (kernel/contract/pluginapi). Nil when unwired — the plugin tool
	// then reports that the host is not available.
	Plugins pluginapi.Spawner
}

// KernelDeps carries the post-Open dependencies for the Configure phase.
type KernelDeps struct {
	K *runtime.Kernel
	// Bus is the publish port the code_exec tool writes its code.executed
	// event through. It is the CONTRACT's port: the tool is a plugin and may
	// not name the platform package that keeps the bus (W3.39f). Nil when
	// unwired — the tool then simply publishes nothing.
	Bus       eventapi.Publisher
	Artifacts artifactapi.Index
	Lake      datalakeapi.Lake
	// Journal is the read port the runs tool recalls the daemon's own past
	// runs through. It is the CONTRACT's port, for the same reason (W3.39f).
	// Nil when unwired — the tool then reports that the run history is
	// unavailable.
	Journal journalapi.Reader
	BaseDir string
	Stdout  io.Writer
	// Reasoning is the reasoning port the council, conductor and research
	// tools drive (W3.39b). It is the contract's port — the tools' runner
	// interfaces live in the contract layer, so the registry can hand one
	// over without naming the live kernel. Nil when unwired: the tools then
	// report that reasoning is unavailable.
	Reasoning reasoningapi.Runner
	// WorkflowRunner is the write-and-run port of the workflow library: the
	// agent authors and runs durable workflows through it (W3.39c). The read
	// side is the store (`Workflows`).
	WorkflowRunner workflowapi.Runner
	// WorkboardRunner is the move-and-claim port of the workboard: the
	// durable, journaled path for typed work items (W3.39c). The read side
	// is the store (`Workboard`).
	WorkboardRunner workboardapi.Runner
	// MCP is the port of the daemon's MCP registry: the servers the agent
	// may add, attach, detach and remove (W3.39d).
	MCP mcpapi.Runner
	// Standing is the port of the daemon's standing orders — the recurring
	// instructions an operator leaves for the fleet (W3.39d).
	Standing standingapi.Host
	// AgentLookup resolves an agent profile by reference. It is a plain
	// closure on purpose: the roster is a module, and a row a plugin names
	// cannot live in a module's implementation package — but a function
	// value names no type at all.
	AgentLookup func(ref string) (agentapi.Profile, bool)
	// Schedules is the daemon's cadence store: the schedule tool writes its
	// entries through the contract's port (W3.39e).
	Schedules cadenceapi.Store
	// SkillForge is the daemon's skills forge: the skill tool authors,
	// promotes and retires its own procedures through the contract's port
	// (W3.39e).
	SkillForge skillapi.Forge
	// SkillBundles is the forge's bundle store — the contract's port, and a
	// nil one leaves the tool reporting that bundles are unavailable.
	SkillBundles skillapi.BundleStore
	// NetguardPublish returns the per-tool OnBlock callback that journals a
	// netguard.blocked event (M109). Supplied by the daemon; Configure wires it
	// into every Netguard spec's instances that implement NetguardAware.
	NetguardPublish func(toolName string) func(ip, reason string)
	// Overseer is the supervisory port the overseer tool steers the fleet
	// through. The tool is a plugin, so it may not reach the live kernel or the
	// board store itself; the daemon supplies the app-layer implementation that
	// answers the contract (kernel/contract/overseerapi). Nil when unwired —
	// the tool then reports that instead of dereferencing nil.
	Overseer overseerapi.Source
	// Introspect is the daemon self-knowledge port the introspect tool reads.
	// The tool is a plugin, so it may not reach the live kernel itself; the
	// daemon supplies the app-layer implementation that answers the contract
	// (kernel/contract/introspectapi). Nil when unwired — the tool then reports
	// that instead of dereferencing nil.
	Introspect introspectapi.Source
	// Forge is the script-tool store port the tool_forge tool reads through
	// (op=list, op=show). The tool is a plugin, so it may not name the
	// module that keeps the store; the daemon supplies it through the
	// contract's port (kernel/contract/toolforgeapi). Nil when unwired — the
	// tool then reports that instead of dereferencing nil.
	Forge toolforgeapi.Store
	// Workflows is the workflow library port the workflow tool reads through
	// (op=list, op=show). The tool is a plugin, so it may not name the
	// module that keeps the store; the daemon supplies it through the
	// contract's port (kernel/contract/workflowapi). Nil when unwired — the
	// tool then reports that instead of dereferencing nil.
	Workflows workflowapi.Store
	// Workboard is the task-store port the workboard tool reads through
	// (op=list, op=show). The tool is a plugin, so it may not name the module
	// that keeps the store; the daemon supplies it through the contract's
	// port (kernel/contract/workboardapi). Nil when unwired — the tool then
	// reports that instead of dereferencing nil.
	Workboard workboardapi.Store
	// Config is the live-daemon port the config tool writes through — the
	// agent-scoped override path and the live-apply reload. The tool is a
	// plugin, so it may not name the runtime; the daemon supplies it through
	// the contract's port (kernel/contract/configapi). Nil when unwired — the
	// tool then reports that instead of dereferencing nil.
	Config configapi.Kernel
}

// LateDeps carries the dependencies for the ConfigureLate phase — wiring that
// needs live channels or the shared message board (built after the kernel).
type LateDeps struct {
	KernelDeps
	ChannelSend      func(ctx context.Context, kind, id, text string) error
	ChannelSendMedia func(ctx context.Context, kind, id, text string, atts []channel.Attachment) error
	// Board is the shared message board the board tool posts through. It is
	// the contract's PORT: the board module's store answers it, and a tool
	// that could name the module's concrete store would reach into it (the
	// board is a module, and the tools are plugins). Nil when unwired — the
	// tool then reports that the board is unavailable.
	Board boardapi.Store
	// BoardNotify is called after each successful post so the host can
	// journal a board.posted event (M656).
	BoardNotify func(m boardapi.Message, corr string)
}

// Built is the result of one Spec's Build. A zero Built (Tool nil, no Extra)
// means the tool declined to register (env-gated off) — not an error.
type Built struct {
	// Tool is the primary instance, keyed in the Set by Definition().Name.
	Tool toolapi.Tool
	// Extra holds additional instances the spec contributes (browser verb
	// tools, plugin tool families), keyed by their registered name.
	Extra map[string]toolapi.Tool
	// Desc is the human "registered" summary line for the boot banner.
	Desc string
	// Caps maps tool name → declared Edict capability (M900 passthrough).
	Caps map[string]string
	// Infos are the plugin-manifest entries for external-plugin specs (one per
	// spawned plugin; the plugin-host spec yields several from a single Build).
	Infos []runtime.PluginInfo
	// Close releases what Build started — the plugin host's child processes.
	// Set.Close calls it once at daemon shutdown (and BuildAll calls it if the
	// set it belonged to fails to build). Nil when there is nothing to release.
	Close func() error
}

// Spec describes one first-party tool: how to build it and which lifecycle
// hooks it participates in. Only Name and Build are required.
type Spec struct {
	Name string
	// Build constructs the tool from boot deps. An error is a hard boot
	// failure; returning a zero Built skips registration.
	Build func(BuildDeps) (Built, error)
	// PreOpen may mutate the runtime Config before runtime.Open.
	//
	// The hook answers the CONTRACT's port, not the daemon's config type: a
	// tool is a plugin and may not name the runtime (which lives in a
	// module). The registry wraps the daemon's configuration in the port
	// before calling the hook (W3.38).
	PreOpen func(tool toolapi.Tool, cfg toolregapi.Config)
	// Configure injects post-Open dependencies.
	Configure func(tool toolapi.Tool, d KernelDeps) error
	// Late injects live-channel / board dependencies.
	Late func(tool toolapi.Tool, d LateDeps) error
	// Netguard marks the spec's instances as egress-guarded: Configure wires
	// NetguardPublish into every instance implementing NetguardAware.
	Netguard bool
	// YieldOnConflict makes this spec's instances LOSE a name collision instead
	// of aborting BuildAll: a tool whose name is already claimed by an
	// earlier-registered spec is dropped (with a warning on BuildDeps.Stderr)
	// and the earlier registration kept. This is the external-plugin-host
	// semantic — in-process tools always win over a plugin's prefixed names —
	// made explicit; first-party specs leave it false so a genuine duplicate
	// stays a hard boot error.
	YieldOnConflict bool
}

// NetguardAware is implemented by tools whose egress guard can report blocked
// dials. Configure calls SetOnBlock with the daemon's audit publisher.
type NetguardAware interface {
	SetOnBlock(func(ip, reason string))
}

// ---- package-level registry -------------------------------------------------

var (
	regMu    sync.RWMutex
	regOrder []string
	registry = map[string]Spec{}
)

// Register adds (or replaces, by Name — so RegisterAll callers are idempotent)
// a spec. Replacing keeps the original registration-order slot.
func Register(sp Spec) {
	regMu.Lock()
	defer regMu.Unlock()
	if _, ok := registry[sp.Name]; !ok {
		regOrder = append(regOrder, sp.Name)
	}
	registry[sp.Name] = sp
}

// Names returns the registered spec names in registration order. It exists
// for the cross-package boot ratchet (plugins/builtintools/ratchet_test.go),
// which pins the exact ordered spec list so the boot tool surface cannot
// change silently. No binary calls it, so deadcodecheck allowlists it by name
// — see tools/deadcodecheck. A same-package accessor would have moved into a
// _test.go instead; this one cannot, because the ratchet lives in the package
// that registers the specs and toolreg cannot import it back.
func Names() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	return append([]string(nil), regOrder...)
}

// snapshot returns the registered specs in registration order.
func snapshot() []Spec {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Spec, 0, len(regOrder))
	for _, name := range regOrder {
		out = append(out, registry[name])
	}
	return out
}

// ---- Set --------------------------------------------------------------------

type pair struct {
	spec  Spec
	built Built
}

// Set holds the (Spec, built instance) pairs produced by BuildAll, and drives
// the lifecycle phases over them in registration order.
type Set struct {
	pairs   []pair
	tools   map[string]toolapi.Tool
	claim   map[string]string // tool name → spec name (collision reporting)
	dropped map[string]bool   // names a YieldOnConflict spec lost to an earlier claimant
}

// BuildAll builds every registered spec against d. A spec whose Build returns
// a zero Built (nil Tool, no Extra, no Infos) is skipped; a Build error
// aborts. Tool names — the primary instance's Definition().Name plus every
// Extra key — must be unique across the set; a collision is an error naming
// both claimants, unless the LATER spec is YieldOnConflict, in which case its
// colliding instance is dropped with a warning on d.Stderr and the earlier
// registration kept (the plugin-host "in-process wins" semantic).
