// SPDX-License-Identifier: MIT
package controlplane

import (
	appseats "github.com/agezt/agezt/kernel/app/seats"
)

// The seats family's ports.
//
// The seat catalog is the DAEMON's own file, and the surface that lists and
// edits it is the daemon's own — so it asks the daemon's kernel, and there is
// no request-level binding: a binding nobody calls is dead code.
//
// The host does not decide what the seats are: it hands over the kernel handle
// and the APPLICATION fills its own ports (`kernel/app/seats/ports.go`). What
// stays here is the binding — which kernel answers the seats family's
// questions.
//
// The handle is never named here: this file does not import the runtime, so the
// adapter may hold the handle without naming the runtime package to hand it
// over.
func (s *Server) seatPorts() appseats.Ports {
	return appseats.PortsFromKernel(s.k)
}
