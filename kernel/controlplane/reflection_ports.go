// SPDX-License-Identifier: MIT
package controlplane

import (
	appreflection "github.com/agezt/agezt/kernel/app/reflection"
)

// The reflection family's ports.
//
// The reflection engine is the DAEMON's own — the composition root always
// builds one — and the surface that triggers and reads a pass is the daemon's
// own, so it asks the daemon's kernel. There is no request-level binding: a
// binding nobody calls is dead code.
//
// The host does not decide what the engine is: it hands over the kernel handle
// and the APPLICATION fills its own ports
// (`kernel/app/reflection/ports.go`). What stays here is the binding — which
// kernel answers the reflection family's questions.
//
// The handle is never named here: this file does not import the runtime, so the
// adapter may hold the handle without naming the runtime package to hand it
// over.
func (s *Server) reflectionPorts() appreflection.Ports {
	return appreflection.PortsFromKernel(s.k)
}
