// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"

	appworkboard "github.com/agezt/agezt/kernel/app/workboard"
	"github.com/agezt/agezt/kernel/runtime"
)

// The workboard application's ports, bound to a kernel.
//
// The host does not decide what the board is read from: it hands over the
// kernel handle and the APPLICATION fills its own ports
// (`kernel/app/workboard/ports.go`). What stays here is the binding — which
// kernel answers the workboard family's questions.
func workboardPortsFor(k *runtime.Kernel) appworkboard.Ports {
	return appworkboard.PortsFromKernel(k)
}

// The daemon's own workboard ports: the root kernel's board.
func (s *Server) workboardPorts() appworkboard.Ports { return workboardPortsFor(s.k) }

// The REQUEST's workboard ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// board is the tenant's board: the routed kernel is the one the request's
// services must read.
func requestWorkboardPorts(ctx context.Context) appworkboard.Ports {
	return workboardPortsFor(ctx.Value(appHostKey{}).(appHost).kernel)
}
