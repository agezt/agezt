// SPDX-License-Identifier: MIT

package approvals

import (
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/runtime"
)

// The approvals family's ports, filled from the kernel handle.
//
// The APPLICATION owns this. The approval registry is the daemon's own
// human-in-the-loop queue — the requests waiting for an operator and the
// decision that answers them — and three surfaces read it: the live list and
// the decision, the mission control's pending count, and the plan runner's
// node spec.
//
// The port holds the kernel's own surface as a CAPABILITY — the registry — so
// a family that asks for the approvals never names the kernel's verb to get
// them.
//
// The kernel may be absent, and the port answers for that case: there is no
// registry to read from, and the surfaces that read it refuse instead of
// reading from nothing.

// Ports are the approvals family's capabilities.
type Ports struct {
	registry func() *approval.Registry
}

// PortsFromKernel fills the approvals family's ports from the kernel handle.
//
// The handle may be nil: a host that holds no kernel holds no approval queue
// either, and the ports say so instead of calling into nothing.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{registry: func() *approval.Registry { return k.Approvals() }}
}

// Broker is the family's own narrow view of the queue: the waiting requests
// and the decision over them.
//
// The name is not "Approvals": a port may not carry the name of the kernel
// verb it stands for, or the adapter that asks the port would be seen reaching
// for the kernel's own verb.
//
// It MAY be absent: the registry is the DAEMON's file, and a host that was
// given none holds none. The surfaces that read it refuse instead of reading
// from nothing.
func (p Ports) Broker() Broker {
	registry := p.Registry()
	if registry == nil {
		return nil
	}
	return registry
}

// Registry is the approval registry itself.
//
// The narrow view is the Broker; the registry is what the SCHEDULER's node
// spec carries, so a family that hands the queue to a plan needs the queue
// itself and not a reader of it.
//
// It MAY be absent, for the same reason the broker may be absent.
func (p Ports) Registry() *approval.Registry {
	if p.registry == nil {
		return nil
	}
	return p.registry()
}
