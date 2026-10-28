// SPDX-License-Identifier: MIT

// Package eventbus is the event bus application's ports.
//
// The bus is PLATFORM (layer 2): the in-process transport every family
// publishes through and subscribes to. What the adapter must not do is reach
// for the kernel's bus on its own — the bus belongs to a family, and a family
// asks the application layer for it.
package eventbus

import (
	"errors"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)

// ErrNoBus is the answer a host with no bus gives.
//
// The adapter used to spell "there is no bus" itself, five times across five
// files, in five spellings that could disagree with each other. The
// application spells it once, here: a host with no bus publishes nothing,
// subscribes to nothing and has no redactor.
var ErrNoBus = errors.New("event bus unavailable")

// Ports are the event bus application's capabilities.
//
// The bus is held UNEXPORTED on purpose. The port is the only way to reach it,
// and every way it offers is nil-safe: the adapter cannot publish into
// nothing, and it cannot forget the check, because there is no check to
// forget.
//
// The bus is the one capability every family PUBLISHES through, and the
// families already narrow it: each consumer declares its OWN view over the
// part of the bus it uses — the roster's operator action, the workboard's
// dispatch, the pulse stream's subscription, the redaction family's redactor.
// The narrow views are the consumers' business, not the family's, so the port
// carries the bus's own operations and every consumer cuts them down to what
// it needs.
type Ports struct {
	bus *bus.Bus
}

// PortsFromKernel fills the event bus application's ports from the kernel
// handle.
//
// The handle may be nil: a host that holds no kernel holds no bus either, and
// the ports say so instead of calling into nothing.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{bus: k.Bus()}
}

// Ready reports whether the host holds a bus at all.
//
// The adapter asks the family this question rather than asking the kernel for
// the bus and testing the answer itself.
func (p Ports) Ready() bool { return p.bus != nil }

// Publish publishes an event.
//
// A host with no bus answers with ErrNoBus and publishes nothing — the same
// answer the adapter's own nil check used to give, spelled once.
func (p Ports) Publish(spec event.Spec) (*event.Event, error) {
	if p.bus == nil {
		return nil, ErrNoBus
	}
	return p.bus.Publish(spec)
}

// Subscribe subscribes to the events matching the pattern.
//
// A host with no bus answers with ErrNoBus and subscribes to nothing.
func (p Ports) Subscribe(pattern string, buffer int) (*bus.Subscription, error) {
	if p.bus == nil {
		return nil, ErrNoBus
	}
	return p.bus.Subscribe(pattern, buffer)
}

// Redactor returns the bus's redactor, or nil when there is no bus.
//
// The redactor scrubs secrets from a payload BEFORE it is written to the
// journal, so the redaction family reads it from here.
func (p Ports) Redactor() bus.Redactor {
	if p.bus == nil {
		return nil
	}
	return p.bus.Redactor()
}
