// SPDX-License-Identifier: MIT

package controlplane

import (
	"github.com/agezt/agezt/kernel/app/binding"
	appcorrelation "github.com/agezt/agezt/kernel/app/correlation"
)

// The daemon's own correlation ports: the root kernel's mint.
//
// The host does not decide what a correlation is: it hands over the kernel
// handle and the APPLICATION fills its own ports
// (`kernel/app/correlation/ports.go`). What stays here is the binding — which
// kernel mints the daemon's correlations.
//
// The handle is never named here: this file does not import the runtime, so
// the adapter may hold the handle without naming the runtime package to hand
// it over.
//
// There is no REQUESTED binding, and the reason is the shape of the request
// path: the router mints the request's correlation BEFORE the request's kernel
// is in the context, and the protocol audit runs on a context that never
// carries one. Both ask the family directly, with the kernel they hold — the
// same shape the council family has always had.
func (s *Server) correlationPorts() appcorrelation.Ports {
	return binding.Fill(s.k, appcorrelation.PortsFromKernel)
}
