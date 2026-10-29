// SPDX-License-Identifier: MIT

package seats

import (
	"github.com/agezt/agezt/kernel/runtime"
)

// The seats family's ports, filled from the kernel handle.
//
// The APPLICATION owns this. The seat catalog is the daemon's own file — the
// execution seats it offers, seeded and custom — and one surface answers from
// it: the catalog.
//
// The port holds the kernel's own surface as a CAPABILITY — the store — so a
// family that asks for the seats never names the kernel's verb to get it.
//
// The kernel may be absent, and the port answers for that case: there is no
// catalog to read from, and the surfaces that read it refuse instead of
// reading from nothing.

// Ports are the seats family's capabilities.
type Ports struct {
	store func() Store
}

// PortsFromKernel fills the seats family's ports from the kernel handle.
//
// The handle may be nil: a host that holds no kernel holds no seat catalog
// either, and the ports say so instead of calling into nothing.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{store: func() Store { return k.Seats() }}
}

// Store is the seat catalog — the execution seats the daemon offers.
//
// The name is not "Seats": a port may not carry the name of the kernel verb it
// stands for, or the adapter that asks the port would be seen reaching for the
// kernel's own verb.
//
// It MAY be absent: the catalog is the DAEMON's file, and a host that was given
// none holds none. The surfaces that read it refuse instead of reading from
// nothing.
func (p Ports) Store() Store {
	if p.store == nil {
		return nil
	}
	return p.store()
}
