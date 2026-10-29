// SPDX-License-Identifier: MIT

package data

import (
	"github.com/agezt/agezt/kernel/runtime"
)

// The data family's ports, filled from the kernel handle.
//
// The APPLICATION owns this. The personal data lake is the daemon's own file —
// the collections the agents build and share — and one surface answers from it:
// the data operations.
//
// The port holds the kernel's own surface as a CAPABILITY — the lake — so a
// family that asks for the lake never names the kernel's verb to get it.
//
// The lake MAY be absent: the lake's files are the composition root's
// business, and a host that was given none holds a nil port and says "not
// available" rather than reading from nothing.

// Ports are the data family's capabilities.
type Ports struct {
	lake func() Lake
}

// PortsFromKernel fills the data family's ports from the kernel handle.
//
// The handle may be nil: a host that holds no kernel holds no lake either, and
// the ports say so instead of calling into nothing.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{lake: func() Lake { return k.DataLake() }}
}

// Lake is the personal data lake — the collections the daemon offers.
//
// The name is not "DataLake": a port may not carry the name of the kernel verb
// it stands for, or the adapter that asks the port would be seen reaching for
// the kernel's own verb.
//
// It MAY be absent: a daemon that was given no lake holds none, and the
// surfaces that read it refuse instead of reading from nothing.
func (p Ports) Lake() Lake {
	if p.lake == nil {
		return nil
	}
	return p.lake()
}
