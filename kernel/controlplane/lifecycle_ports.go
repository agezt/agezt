// SPDX-License-Identifier: MIT
package controlplane

import (
	applifecycle "github.com/agezt/agezt/kernel/app/lifecycle"
)

// The daemon lifecycle family's ports.
//
// The daemon's lifecycle capability — drain the runs in flight and halt — is
// the DAEMON's own, and the flow that asks for it (the update) asks the
// daemon's kernel. There is no request-level binding: a binding nobody calls
// is dead code.
//
// The host does not decide how a daemon is drained: it hands over the kernel
// handle and the APPLICATION fills its own ports
// (`kernel/app/lifecycle/ports.go`). What stays here is the binding — which
// kernel answers the lifecycle family's questions.
//
// The handle is never named here: this file does not import the runtime, so the
// adapter may hold the handle without naming the runtime package to hand it
// over.
func (s *Server) lifecyclePorts() applifecycle.Ports {
	return applifecycle.PortsFromKernel(s.k)
}
