// SPDX-License-Identifier: MIT

package controlplane

import (
	appcorrelation "github.com/agezt/agezt/kernel/app/correlation"
)

// The daemon's own correlation ports: the root kernel's mint.
//
// The host does not decide what a correlation is: the COMPOSITION ROOT fills the
// scope and the APPLICATION fills the family's own ports
// (`kernel/app/correlation/ports.go`). What stays here is the binding — which
// kernel mints the daemon's correlations.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
//
// There is no REQUESTED binding, and the reason is the shape of the request
// path: the router mints the request's correlation BEFORE the request's kernel
// is in the context, and the protocol audit runs on a context that never
// carries one. Both ask the family directly, with the kernel they hold — the
// same shape the council family has always had.
func (s *Server) correlationPorts() appcorrelation.Ports {
	return s.scoped().CorrelationPorts
}
