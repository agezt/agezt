// SPDX-License-Identifier: MIT

package provider

import (
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/runtime"
)

// The live provider's ports, filled from the kernel handle.
//
// The APPLICATION owns this. The daemon's live provider is the door several
// families ask through — the routing tables it routes by, the spend it keeps
// and the routing view it reports — and the application is what knows which
// face each of them needs. The host hands over the handle and nothing else.
//
// The provider is CONTRACT (llm.Provider), and the daemon's live provider is
// the governor: it carries the routing tables and the spend. The port names
// those faces as CAPABILITIES, so a family that asks for the chains never
// learns what the governor is.
//
// The provider may be absent — a host that was given none holds a nil
// provider — and the port answers for that case: the chains are absent, the
// routing view is absent, the budget is absent. Nothing is invented, and
// nothing panics.

// Ports are the live provider's capabilities.
type Ports struct {
	provider llm.Provider
}

// PortsFromKernel fills the live provider's ports from the kernel handle.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{provider: k.Provider()}
}

// Chains are the provider's routing tables: the task-model chains it routes
// by and the named fallback chains it falls back to.
//
// The tables are the provider's own; a family that edits them edits the
// routing the daemon actually takes.
type Chains interface {
	TaskModelChainsView() map[string][]string
	SetTaskModelChains(map[string][]string)
	FallbackChainsView() (map[string][]string, string)
	SetFallbackChains(map[string][]string, string)
}

// Routing is the provider's routing view: the routes, the routes that require
// an approval, and the per-task model overrides.
type Routing interface {
	TaskRoutesView() map[string][]string
	TaskRouteRequiresView() map[string][]string
	TaskModelOverridesView() map[string]string
}

// Budget is the provider's spend keeper.
//
// The ceiling's answer is the provider's own — how much of the ceiling is
// left — and a family that does not report it may drop it.
type Budget interface {
	Snapshot() governor.BudgetSnapshot
	SetDailyCeiling(microcents int64) int64
}

// Live is the live provider itself.
//
// Two families need the provider and not one of its faces: the chat family
// summarizes through it and the runs family authors plans through it. They ask
// for the provider, and the port hands it over — the object is the contract's
// (llm.Provider), and a family that drives a completion needs the contract.
//
// The name is not "Provider": a port may not carry the name of the kernel verb
// it stands for, or the adapter that asks the port would be seen reaching for
// the kernel's own verb.
func (p Ports) Live() llm.Provider { return p.provider }

// chains returns the provider's routing tables, if it carries them.
func (p Ports) chains() (Chains, bool) {
	if p.provider == nil {
		return nil, false
	}
	c, ok := p.provider.(Chains)
	return c, ok
}

// TaskChains are the provider's task-model chains.
//
// A provider that carries no chains answers false: the tables are absent, not
// empty.
func (p Ports) TaskChains() (map[string][]string, bool) {
	c, ok := p.chains()
	if !ok {
		return nil, false
	}
	return c.TaskModelChainsView(), true
}

// SetTaskChains replaces the provider's task-model chains.
//
// A provider that carries no chains keeps the ones it has: there are none,
// and there is nothing to replace.
func (p Ports) SetTaskChains(chains map[string][]string) {
	if c, ok := p.chains(); ok {
		c.SetTaskModelChains(chains)
	}
}

// Named are the provider's named fallback chains and the default chain name.
func (p Ports) Named() (map[string][]string, string, bool) {
	c, ok := p.chains()
	if !ok {
		return nil, "", false
	}
	chains, def := c.FallbackChainsView()
	return chains, def, true
}

// SetNamed replaces the provider's named fallback chains and the default
// chain name.
func (p Ports) SetNamed(chains map[string][]string, def string) {
	if c, ok := p.chains(); ok {
		c.SetFallbackChains(chains, def)
	}
}

// Routing is the provider's routing view, if it carries one.
func (p Ports) Routing() (Routing, bool) {
	if p.provider == nil {
		return nil, false
	}
	r, ok := p.provider.(Routing)
	return r, ok
}

// Budget is the provider's spend keeper, if it carries one.
//
// The keeper is nil when the provider does not keep a budget: a host without
// a governor spends nothing, and the budget family answers for that.
func (p Ports) Budget() Budget {
	if p.provider == nil {
		return nil
	}
	b, ok := p.provider.(Budget)
	if !ok {
		return nil
	}
	return b
}

// SpentMicrocents is what the provider has spent today.
//
// A provider that keeps no budget has spent nothing — and says so with false,
// because "nothing spent" and "no budget at all" are different answers.
func (p Ports) SpentMicrocents() (int64, bool) {
	b := p.Budget()
	if b == nil {
		return 0, false
	}
	return b.Snapshot().SpentMicrocents, true
}
