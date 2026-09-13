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

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/datalake"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/warden"
)


// BuildDeps carries everything a tool may need at pre-kernel construction
// time. Env is read through Get (injectable for tests); AllowAll is the master
// permissive switch (AGEZT_ALLOW_ALL=1) the daemon resolves once.
type BuildDeps struct {
	BaseDir       string
	WorkspaceRoot string
	Warden        warden.Engine
	Stderr        io.Writer
	Get           func(name string) string
	AllowAll      bool
	NotifyTargets map[string][]string
}

// KernelDeps carries the post-Open dependencies for the Configure phase.
type KernelDeps struct {
	K         *runtime.Kernel
	Bus       *bus.Bus
	Artifacts *artifact.Index
	Lake      *datalake.Lake
	Journal   *journal.Journal
	BaseDir   string
	Stdout    io.Writer
	// NetguardPublish returns the per-tool OnBlock callback that journals a
	// netguard.blocked event (M109). Supplied by the daemon; Configure wires it
	// into every Netguard spec's instances that implement NetguardAware.
	NetguardPublish func(toolName string) func(ip, reason string)
}

// LateDeps carries the dependencies for the ConfigureLate phase — wiring that
// needs live channels or the shared message board (built after the kernel).
type LateDeps struct {
	KernelDeps
	ChannelSend      func(ctx context.Context, kind, id, text string) error
	ChannelSendMedia func(ctx context.Context, kind, id, text string, atts []channel.Attachment) error
	Board            *board.Store
	BoardNotify      func(m board.Message, corr string)
}

// Built is the result of one Spec's Build. A zero Built (Tool nil, no Extra)
// means the tool declined to register (env-gated off) — not an error.
type Built struct {
	// Tool is the primary instance, keyed in the Set by Definition().Name.
	Tool agent.Tool
	// Extra holds additional instances the spec contributes (browser verb
	// tools, plugin tool families), keyed by their registered name.
	Extra map[string]agent.Tool
	// Desc is the human "registered" summary line for the boot banner.
	Desc string
	// Caps maps tool name → declared Edict capability (M900 passthrough).
	Caps map[string]string
	// Infos are the plugin-manifest entries for external-plugin specs (one per
	// spawned plugin; the plugin-host spec yields several from a single Build).
	Infos []runtime.PluginInfo
}

// Spec describes one first-party tool: how to build it and which lifecycle
// hooks it participates in. Only Name and Build are required.
type Spec struct {
	Name string
	// Build constructs the tool from boot deps. An error is a hard boot
	// failure; returning a zero Built skips registration.
	Build func(BuildDeps) (Built, error)
	// PreOpen may mutate the runtime Config before runtime.Open.
	PreOpen func(tool agent.Tool, cfg *runtime.Config)
	// Configure injects post-Open dependencies.
	Configure func(tool agent.Tool, d KernelDeps) error
	// Late injects live-channel / board dependencies.
	Late func(tool agent.Tool, d LateDeps) error
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
	tools   map[string]agent.Tool
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
