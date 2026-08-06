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
	"fmt"
	"io"
	"sort"
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
	// Info is the plugin-manifest entry for external-plugin specs.
	Info *runtime.PluginInfo
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

// Lookup returns the spec registered under name.
func Lookup(name string) (Spec, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	sp, ok := registry[name]
	return sp, ok
}

// Names returns the registered spec names in registration order.
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
	pairs []pair
	tools map[string]agent.Tool
	claim map[string]string // tool name → spec name (collision reporting)
}

// BuildAll builds every registered spec against d. A spec whose Build returns
// a zero Built (nil Tool, no Extra) is skipped; a Build error aborts. Tool
// names — the primary instance's Definition().Name plus every Extra key — must
// be unique across the set; a collision is an error naming both claimants.
func BuildAll(d BuildDeps) (*Set, error) {
	s := &Set{tools: map[string]agent.Tool{}, claim: map[string]string{}}
	for _, sp := range snapshot() {
		if sp.Build == nil {
			continue
		}
		b, err := sp.Build(d)
		if err != nil {
			return nil, fmt.Errorf("tool %s: %w", sp.Name, err)
		}
		if b.Tool == nil && len(b.Extra) == 0 {
			continue // env-gated off
		}
		if b.Tool != nil {
			if err := s.add(b.Tool.Definition().Name, b.Tool, sp.Name); err != nil {
				return nil, err
			}
		}
		for _, name := range sortedKeys(b.Extra) {
			if err := s.add(name, b.Extra[name], sp.Name); err != nil {
				return nil, err
			}
		}
		s.pairs = append(s.pairs, pair{spec: sp, built: b})
	}
	return s, nil
}

func (s *Set) add(name string, tl agent.Tool, specName string) error {
	if prev, dup := s.claim[name]; dup {
		return fmt.Errorf("tool name %q claimed by both spec %s and spec %s", name, prev, specName)
	}
	s.claim[name] = specName
	s.tools[name] = tl
	return nil
}

// Tools returns the merged name → instance map (a copy; instances are shared).
func (s *Set) Tools() map[string]agent.Tool {
	out := make(map[string]agent.Tool, len(s.tools))
	for name, tl := range s.tools {
		out[name] = tl
	}
	return out
}

// ApplyPreOpen runs every spec's PreOpen hook against cfg.
func (s *Set) ApplyPreOpen(cfg *runtime.Config) {
	for _, p := range s.pairs {
		if p.spec.PreOpen != nil {
			p.spec.PreOpen(p.built.Tool, cfg)
		}
	}
}

// Configure runs the post-Open phase: for every Netguard spec it wires
// d.NetguardPublish into each instance implementing NetguardAware (primary
// first, then Extras in sorted-name order), then invokes the spec's Configure
// hook. Pairs run in registration order.
func (s *Set) Configure(d KernelDeps) error {
	for _, p := range s.pairs {
		if p.spec.Netguard && d.NetguardPublish != nil {
			if na, ok := p.built.Tool.(NetguardAware); ok {
				na.SetOnBlock(d.NetguardPublish(p.built.Tool.Definition().Name))
			}
			for _, name := range sortedKeys(p.built.Extra) {
				if na, ok := p.built.Extra[name].(NetguardAware); ok {
					na.SetOnBlock(d.NetguardPublish(name))
				}
			}
		}
		if p.spec.Configure != nil {
			if err := p.spec.Configure(p.built.Tool, d); err != nil {
				return fmt.Errorf("configure %s: %w", p.spec.Name, err)
			}
		}
	}
	return nil
}

// ConfigureLate runs the live-channel/board phase in registration order.
func (s *Set) ConfigureLate(d LateDeps) error {
	for _, p := range s.pairs {
		if p.spec.Late != nil {
			if err := p.spec.Late(p.built.Tool, d); err != nil {
				return fmt.Errorf("late-configure %s: %w", p.spec.Name, err)
			}
		}
	}
	return nil
}

// Descs returns each built spec's banner line, in registration order,
// skipping empties.
func (s *Set) Descs() []string {
	var out []string
	for _, p := range s.pairs {
		if p.built.Desc != "" {
			out = append(out, p.built.Desc)
		}
	}
	return out
}

// PluginManifest collects the external-plugin manifest entries.
func (s *Set) PluginManifest() []runtime.PluginInfo {
	var out []runtime.PluginInfo
	for _, p := range s.pairs {
		if p.built.Info != nil {
			out = append(out, *p.built.Info)
		}
	}
	return out
}

// ToolCapabilities merges every built spec's declared capability map (M900).
func (s *Set) ToolCapabilities() map[string]string {
	var out map[string]string
	for _, p := range s.pairs {
		for name, cap := range p.built.Caps {
			if out == nil {
				out = map[string]string{}
			}
			out[name] = cap
		}
	}
	return out
}

// NetguardGaps returns the names of built Netguard specs none of whose
// instances implement NetguardAware — i.e. egress-guarded tools whose SSRF
// refusals would go unjournaled. A registry-driven guard test asserts this is
// empty, replacing the old hand-listed type switch. Specs that skipped
// themselves at Build are not gaps (nothing was built to wire).
func (s *Set) NetguardGaps() []string {
	var gaps []string
	for _, p := range s.pairs {
		if !p.spec.Netguard {
			continue
		}
		aware := false
		if _, ok := p.built.Tool.(NetguardAware); ok {
			aware = true
		}
		for _, tl := range p.built.Extra {
			if _, ok := tl.(NetguardAware); ok {
				aware = true
				break
			}
		}
		if !aware {
			gaps = append(gaps, p.spec.Name)
		}
	}
	return gaps
}

func sortedKeys(m map[string]agent.Tool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
