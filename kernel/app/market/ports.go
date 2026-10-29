// SPDX-License-Identifier: MIT
package market

import (
	"github.com/agezt/agezt/kernel/runtime"
)

// The marketplace family's ports, filled from the kernel handle.
//
// The APPLICATION owns this. The marketplace is the daemon's own library of
// packs — the built-in Official library and the sources the operator adds — and
// it is the DAEMON's file: the kernel holds the CONTRACT's manager port.
//
// The port holds the kernel's own surface as a CAPABILITY — the manager — so a
// family that asks for the marketplace never names the kernel's verb to get it.
//
// The kernel may be absent, and the port answers for that case: there is no
// manager to read from, and the surfaces that read it refuse instead of reading
// from nothing.

// Manager is the marketplace manager the daemon holds — the reads and the
// writes of one manager, declared as ONE surface so a family asks for the whole
// and the application cuts it down: the reads take the reading half, the writes
// the writing half.
type Manager interface {
	Reader
	Writer
}

// Ports are the marketplace family's capabilities.
type Ports struct {
	manager func() Manager
}

// PortsFromKernel fills the marketplace family's ports from the kernel handle.
//
// The handle may be nil: a host that holds no kernel holds no marketplace
// either, and the ports say so instead of calling into nothing.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{manager: func() Manager { return k.Market() }}
}

// Manager is the marketplace manager the daemon holds.
//
// The name is not "Market": a port may not carry the name of the kernel verb it
// stands for, or the adapter that asks the port would be seen reaching for
// the kernel's own verb.
//
// A host with no kernel holds no marketplace, and the port hands over nothing:
// the surfaces that read it refuse instead of reading from nothing.
func (p Ports) Manager() Manager {
	if p.manager == nil {
		return nil
	}
	return p.manager()
}
