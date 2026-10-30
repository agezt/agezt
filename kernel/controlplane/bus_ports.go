// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"

	"github.com/agezt/agezt/kernel/app/binding"
	appeventbus "github.com/agezt/agezt/kernel/app/eventbus"
)

// The daemon's own bus ports: the root kernel's event bus.
//
// The host does not decide what the bus is: it hands over the kernel handle
// and the APPLICATION fills its own ports
// (`kernel/app/eventbus/ports.go`). What stays here is the binding — which
// kernel answers the bus family's questions.
//
// The handle is never named here: this file does not import the runtime, so
// the adapter may hold the handle without naming the runtime package to hand
// it over.
func (s *Server) busPorts() appeventbus.Ports { return binding.Fill(s.k, appeventbus.PortsFromKernel) }

// The REQUEST's bus ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's bus
// is the tenant's bus: the routed kernel is the one the request's publishers
// must write to. The daemon's own ports answer for the root kernel.
func requestBusPorts(ctx context.Context) appeventbus.Ports {
	return binding.Fill(ctx.Value(appHostKey{}).(appHost).kernel, appeventbus.PortsFromKernel)
}
