// SPDX-License-Identifier: MIT
package controlplane

import (
	"github.com/agezt/agezt/kernel/app/binding"
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
// The host does not decide how a run is started: it hands over the kernel
// handle and the APPLICATION fills its own ports (`kernel/app/runs/ports.go`).
// What stays here is the binding — which kernel answers the runs family's
// questions.
//
// The handle is never named here: this file does not import the runtime, so the
// adapter may hold the handle without naming the runtime package to hand it
// over.
func (s *Server) runPorts() appruns.Ports {
	return binding.Fill(s.k, appruns.PortsFromKernel)
}
