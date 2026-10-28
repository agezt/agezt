// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"

	apptools "github.com/agezt/agezt/kernel/app/tools"
)

// The daemon's own tools ports: the root kernel's registry, forge and MCP
// servers.
//
// The host does not decide what the tools are read from: it hands over the
// kernel handle and the APPLICATION fills its own ports
// (`kernel/app/tools/ports.go`). What stays here is the binding — which
// kernel answers the tools family's questions.
//
// The handle is never named here: this file does not import the runtime, so
// the adapter may hold the handle without naming the runtime package to hand
// it over.
func (s *Server) toolsPorts() apptools.Ports { return apptools.PortsFromKernel(s.k) }

// The REQUEST's tools ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// tools are the tenant's tools: the routed kernel is the one the request's
// services must read. The daemon's own ports answer for the root kernel.
func requestToolsPorts(ctx context.Context) apptools.Ports {
	return apptools.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
