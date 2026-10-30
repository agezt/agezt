// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"

	"github.com/agezt/agezt/kernel/app/binding"
	appworkboard "github.com/agezt/agezt/kernel/app/workboard"
)

// The daemon's own workboard ports: the root kernel's board.
//
// The host does not decide what the board is read from: it hands over the
// kernel handle and the APPLICATION fills its own ports
// (`kernel/app/workboard/ports.go`). What stays here is the binding — which
// kernel answers the workboard family's questions.
//
// The handle is never named here: this file does not import the runtime, so
// the adapter may hold the handle without naming the runtime package to hand
// it over.
func (s *Server) workboardPorts() appworkboard.Ports {
	return binding.Fill(s.k, appworkboard.PortsFromKernel)
}

// The REQUEST's workboard ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// board is the tenant's board: the routed kernel is the one the request's
// services must read.
func requestWorkboardPorts(ctx context.Context) appworkboard.Ports {
	return binding.Fill(ctx.Value(appHostKey{}).(appHost).kernel, appworkboard.PortsFromKernel)
}
