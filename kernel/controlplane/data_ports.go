// SPDX-License-Identifier: MIT
package controlplane

import (
	appdata "github.com/agezt/agezt/kernel/app/data"
)

// The data family's ports.
//
// The lake is the DAEMON's own file — the collections the agents build and
// share — and the surface that lists and edits it is the daemon's own — so it
// asks the daemon's kernel, and there is no request-level binding: a binding
// nobody calls is dead code.
//
// The host does not decide what the lake is: it hands over the kernel handle
// and the APPLICATION fills its own ports (`kernel/app/data/ports.go`). What
// stays here is the binding — which kernel answers the data family's
// questions.
//
// The handle is never named here: this file does not import the runtime, so the
// adapter may hold the handle without naming the runtime package to hand it
// over.
func (s *Server) dataPorts() appdata.Ports {
	return appdata.PortsFromKernel(s.k)
}
