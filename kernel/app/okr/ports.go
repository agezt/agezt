// SPDX-License-Identifier: MIT

package okr

import (
	"github.com/agezt/agezt/kernel/runtime"
)

// The objective application's ports, filled from the kernel handle.
//
// The APPLICATION owns this: it knows which capabilities the objectives need
// and how each of them answers. The host hands over the handle and nothing
// else — it does not decide what the objectives are read from, and it cannot
// change the objective wiring without editing the application.
//
// The same shape already exists in app/council, app/roster, app/standing,
// app/schedule, app/workflow and app/tools: the application takes the kernel
// and builds its own ports.
//
// The objective library is the DAEMON's (W3.53): the kernel holds the
// contract's port, and a host that was given none holds a nil port. The
// reader port is that port, and it is nil when there is no library — the
// application refuses rather than reading into nothing.

// Ports are the objective application's capabilities over the objectives.
type Ports struct {
	// Reader reads the objective library.
	//
	// It is the CONTRACT's store port: the daemon opens the file and hands
	// the port over. It is NIL when the daemon holds no objective library —
	// the application answers "the objective library is not available on
	// this daemon" rather than calling into nothing.
	Reader Reader

	// Progress answers the rollup of an objective's progress.
	//
	// The field is the application's own name: the kernel's verb for the
	// same thing is an objective verb, and the adapter must not name it —
	// not even as a field of the port it holds.
	Progress Rollupper

	// Mutation writes the objectives: create, key result, link, unlink and
	// archive.
	Mutation MutationHost
}

// PortsFromKernel fills the objective application's ports from the kernel
// handle.
func PortsFromKernel(k *runtime.Kernel) Ports {
	return Ports{
		Reader:   k.OKR(),
		Progress: k,
		Mutation: k,
	}
}
