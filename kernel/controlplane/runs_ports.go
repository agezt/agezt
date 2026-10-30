// SPDX-License-Identifier: MIT
package controlplane

import (
	appruns "github.com/agezt/agezt/kernel/app/runs"
)

// The runs family's ports.
//
// The daemon's run capability is the DAEMON's own — it starts a run under a
// correlation and returns its answer — and the two surfaces that ask for it
// (the roster's operator wake and the workboard's dispatch) are the daemon's
// own, so they ask the daemon's kernel. There is no request-level binding: a
// binding nobody calls is dead code.
//
// The host does not decide how a run is started: the COMPOSITION ROOT fills the
// scope and the APPLICATION fills the family's own ports (`kernel/app/runs/ports.go`).
// What stays here is the binding — which kernel answers the runs family's
// questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) runPorts() appruns.Ports {
	return s.scoped().RunsPorts
}
