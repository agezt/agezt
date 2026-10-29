// SPDX-License-Identifier: MIT

package taste

import (
	"github.com/agezt/agezt/kernel/contract/tasteapi"
	"github.com/agezt/agezt/kernel/runtime"
)

// The taste family's ports, filled from the kernel handle.
//
// The APPLICATION owns this. The taste is the daemon's own library of curated
// exemplars — the examples a run may imitate — and one surface answers from
// it: the library.
//
// The port holds the kernel's own surface as a CAPABILITY — the store — so a
// family that asks for the taste never names the kernel's verb to get it.
//
// The kernel may be absent, and the port answers for that case: there is no
// library to read from, and the surfaces that read it report the empty library
// instead of reading from nothing.

// Ports are the taste family's capabilities.
type Ports struct {
	store func() tasteapi.Store
}

// PortsFromKernel fills the taste family's ports from the kernel handle.
//
// The handle may be nil: a host that holds no kernel holds no taste library
// either, and the ports say so instead of calling into nothing.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{store: func() tasteapi.Store { return k.Taste() }}
}

// Store is the taste library — the curated exemplars the daemon keeps.
//
// The name is not "Taste": a port may not carry the name of the kernel verb it
// stands for, or the adapter that asks the port would be seen reaching for the
// kernel's own verb.
//
// It MAY be absent: the library is the DAEMON's file, and a host that was
// given none holds none. The surfaces that read it report the empty library
// instead of reading from nothing.
func (p Ports) Store() tasteapi.Store {
	if p.store == nil {
		return nil
	}
	return p.store()
}
