// SPDX-License-Identifier: MIT

package reflection

import (
	"github.com/agezt/agezt/kernel/runtime"
)

// The reflection family's ports, filled from the kernel handle.
//
// The APPLICATION owns this. The reflection engine is the daemon's own — the
// pass that folds the journal into observations, applies the world model's
// decay and derives advisory proposals — and one surface answers from it: the
// reflection operations.
//
// The port holds the kernel's own surface as a CAPABILITY — the engine — so a
// family that asks for the engine never names the kernel's verb to get it.
//
// The engine is NOT a daemon's file: the composition root always builds one,
// so a daemon that was given a kernel always holds an engine. The port's
// absent case is the host that holds no kernel at all — and the port says so
// instead of calling into nothing.

// Ports are the reflection family's capabilities.
type Ports struct {
	engine func() Engine
}

// PortsFromKernel fills the reflection family's ports from the kernel handle.
//
// The handle may be nil: a host that holds no kernel holds no reflection
// engine either, and the ports say so instead of calling into nothing.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{engine: func() Engine { return k.Reflect() }}
}

// Engine is the reflection engine — the pass that explains the world model.
//
// The name is not "Reflect": a port may not carry the name of the kernel verb
// it stands for, or the adapter that asks the port would be seen reaching for
// the kernel's own verb.
//
// It MAY be absent: a host that holds no kernel holds none, and the surfaces
// that read it refuse instead of reading from nothing.
func (p Ports) Engine() Engine {
	if p.engine == nil {
		return nil
	}
	return p.engine()
}
