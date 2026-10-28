// SPDX-License-Identifier: MIT

package edict

import (
	core "github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/runtime"
)

// The policy family's ports, filled from the kernel handle.
//
// The APPLICATION owns this: it knows which capabilities the policy engine
// needs and how each of them answers. The host hands over the handle and
// nothing else.
//
// The engine is PLATFORM: the kernel always opens one, and a host that was
// given none holds a nil engine. The policy family's answer for that case is a
// REFUSAL — a decision that says no — not a panic.

// Ports are the policy family's capabilities over the daemon's policy engine.
type Ports struct {
	engine *core.Engine
}

// PortsFromKernel fills the policy family's ports from the kernel handle.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{engine: k.Edict()}
}

// Engine is the read view the edict service takes.
//
// It is nil when the host holds no engine, and the service already knows
// this: it refuses a missing engine instead of panicking.
func (p Ports) Engine() Engine {
	if p.engine == nil {
		return nil
	}
	return p.engine
}

// WriteEngine is the write view the writes service takes.
func (p Ports) WriteEngine() WriteEngine {
	if p.engine == nil {
		return nil
	}
	return p.engine
}

// DecideWithCeiling decides a capability under a trust ceiling.
//
// A host with no engine decides nothing, and the refusal is a DENY: a policy
// that cannot be asked must not let a capability through.
func (p Ports) DecideWithCeiling(capability core.Capability, input string, ceiling core.TrustLevel) core.Outcome {
	if p.engine == nil {
		return core.Outcome{
			Decision:   core.DecisionDeny,
			Capability: capability,
			Reason:     "no policy engine on this daemon",
		}
	}
	return p.engine.DecideWithCeiling(capability, input, ceiling)
}

// AskPolicy is the engine's ask-policy answer.
//
// A host with no engine asks nothing: the empty answer is the answer.
func (p Ports) AskPolicy() string {
	if p.engine == nil {
		return ""
	}
	return p.engine.AskPolicy().String()
}
