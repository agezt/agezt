// SPDX-License-Identifier: MIT
package controlplane

import (
	appstate "github.com/agezt/agezt/kernel/app/state"
)

// The state family's ports.
//
// The state surface is the DAEMON's own: it inspects the daemon's key/value
// file, so it asks the daemon's kernel — and there is no request-level
// binding: a binding nobody calls is dead code.
//
// The host does not decide what the state is: the COMPOSITION ROOT fills the scope
// and the APPLICATION fills the family's own ports (`kernel/app/state/ports.go`). What
// stays here is the binding — which kernel answers the state family's
// questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) statePorts() appstate.Ports {
	return s.scoped().StatePorts
}
