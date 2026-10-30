// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"

	"github.com/agezt/agezt/kernel/app/binding"
	appedict "github.com/agezt/agezt/kernel/app/edict"
)

// The daemon's own policy ports: the root kernel's policy engine.
//
// The host does not decide what the policy engine is: it hands over the kernel
// handle and the APPLICATION fills its own ports
// (`kernel/app/edict/ports.go`). What stays here is the binding — which kernel
// answers the policy family's questions.
//
// The handle is never named here: this file does not import the runtime, so
// the adapter may hold the handle without naming the runtime package to hand
// it over.
func (s *Server) edictPorts() appedict.Ports { return binding.Fill(s.k, appedict.PortsFromKernel) }

// The REQUEST's policy ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// policy engine is the tenant's engine: the routed kernel is the one the
// request's services must read. The daemon's own ports answer for the root
// kernel.
func requestEdictPorts(ctx context.Context) appedict.Ports {
	return binding.Fill(ctx.Value(appHostKey{}).(appHost).kernel, appedict.PortsFromKernel)
}
