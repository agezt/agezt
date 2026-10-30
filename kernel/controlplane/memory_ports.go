// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"

	appmemory "github.com/agezt/agezt/kernel/app/memory"
)

// The daemon's own memory ports: the root kernel's memory.
//
// The host does not decide what the memory is: the COMPOSITION ROOT fills the
// scope and the APPLICATION fills the family's own ports
// (`kernel/app/memory/ports.go`). What stays here is the reading — which
// scope answers the memory family's questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) memoryPorts() appmemory.Ports { return s.scoped().MemoryPorts }

// The REQUEST's memory ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// memory is the tenant's memory: the routed kernel is the one the request's
// services must read. The daemon's own ports answer for the root kernel.
func requestMemoryPorts(ctx context.Context) appmemory.Ports {
	return requestScope(ctx).MemoryPorts
}
