// SPDX-License-Identifier: MIT
package controlplane

import (
	"github.com/agezt/agezt/kernel/app/binding"
	appconfigcenter "github.com/agezt/agezt/kernel/app/configcenter"
)

// The config center family's ports, bound to this daemon's kernel.
//
// The host does not decide what the config center is read from: it hands over
// the kernel handle and the APPLICATION fills its own ports
// (`kernel/app/configcenter/ports.go`). What stays here is the binding — which
// kernel answers the config center family's questions.
//
// The handle is never named here: this file does not import the runtime, so
// the adapter may hold the handle without naming the runtime package to hand
// it over.
func (s *Server) configCenterPorts() appconfigcenter.Ports {
	return binding.Fill(s.k, appconfigcenter.PortsFromKernel)
}
