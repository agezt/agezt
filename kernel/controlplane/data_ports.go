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
// The host does not decide what the lake is: the COMPOSITION ROOT fills the scope
// and the APPLICATION fills the family's own ports (`kernel/app/data/ports.go`). What
// stays here is the binding — which kernel answers the data family's
// questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) dataPorts() appdata.Ports {
	return s.scoped().DataPorts
}
