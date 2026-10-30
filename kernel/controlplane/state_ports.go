// SPDX-License-Identifier: MIT
package controlplane

import (
	"github.com/agezt/agezt/kernel/app/binding"
	appstate "github.com/agezt/agezt/kernel/app/state"
)

// The state family's ports.
//
// The state surface is the DAEMON's own: it inspects the daemon's key/value
// file, so it asks the daemon's kernel — and there is no request-level
// binding: a binding nobody calls is dead code.
//
// The host does not decide what the state is: it hands over the kernel handle
// and the APPLICATION fills its own ports (`kernel/app/state/ports.go`). What
// stays here is the binding — which kernel answers the state family's
// questions.
//
// The handle is never named here: this file does not import the runtime, so the
// adapter may hold the handle without naming the runtime package to hand it
// over.
func (s *Server) statePorts() appstate.Ports {
	return binding.Fill(s.k, appstate.PortsFromKernel)
}
