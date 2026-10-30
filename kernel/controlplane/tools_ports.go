// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"

	apptools "github.com/agezt/agezt/kernel/app/tools"
)

// The daemon's own tools ports: the root kernel's registry, forge and MCP
// servers.
//
// The host does not decide what the tools are read from: the COMPOSITION ROOT fills the
// scope and the APPLICATION fills the family's own ports
// (`kernel/app/tools/ports.go`). What stays here is the reading — which
// scope answers the tools family's questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) toolsPorts() apptools.Ports { return s.scoped().ToolsPorts }

// The REQUEST's tools ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// tools are the tenant's tools: the routed kernel is the one the request's
// services must read. The daemon's own ports answer for the root kernel.
func requestToolsPorts(ctx context.Context) apptools.Ports {
	return requestScope(ctx).ToolsPorts
}
