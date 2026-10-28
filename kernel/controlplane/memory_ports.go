// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"

	appmemory "github.com/agezt/agezt/kernel/app/memory"
)

// The daemon's own memory ports: the root kernel's memory.
//
// The host does not decide what the memory is: it hands over the kernel
// handle and the APPLICATION fills its own ports
// (`kernel/app/memory/ports.go`). What stays here is the binding — which
// kernel answers the memory family's questions.
//
// The handle is never named here: this file does not import the runtime, so
// the adapter may hold the handle without naming the runtime package to hand
// it over.
func (s *Server) memoryPorts() appmemory.Ports { return appmemory.PortsFromKernel(s.k) }

// The REQUEST's memory ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// memory is the tenant's memory: the routed kernel is the one the request's
// services must read. The daemon's own ports answer for the root kernel.
func requestMemoryPorts(ctx context.Context) appmemory.Ports {
	return appmemory.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
