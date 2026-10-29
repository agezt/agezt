// SPDX-License-Identifier: MIT

package state

import (
	"github.com/agezt/agezt/kernel/runtime"
)

// The state family's ports, filled from the kernel handle.
//
// The APPLICATION owns this. The state store is the daemon's own key/value
// file — the namespaces and keys the daemon keeps — and one surface answers
// from it: the inspection surface.
//
// The port holds the kernel's own surface as a CAPABILITY — the store — so a
// family that asks for the state never names the kernel's verb to get it.
//
// The kernel may be absent, and the port answers for that case: there is no
// store to read from, and the surfaces that read it refuse instead of reading
// from nothing.

// Ports are the state family's capabilities.
type Ports struct {
	store func() Store
}

// PortsFromKernel fills the state family's ports from the kernel handle.
//
// The handle may be nil: a host that holds no kernel holds no state store
// either, and the ports say so instead of calling into nothing.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{store: func() Store { return k.State() }}
}

// Store is the daemon's key/value store — the namespaces and the keys under
// them.
//
// The name is not "State": a port may not carry the name of the kernel verb it
// stands for, or the adapter that asks the port would be seen reaching for the
// kernel's own verb.
//
// It MAY be absent: the store is the DAEMON's file, and a host that was given
// none holds none. The surfaces that read it refuse instead of reading from
// nothing.
func (p Ports) Store() Store {
	if p.store == nil {
		return nil
	}
	return p.store()
}
