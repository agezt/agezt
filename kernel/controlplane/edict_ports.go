// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"

	appedict "github.com/agezt/agezt/kernel/app/edict"
)

// The daemon's own policy ports: the root kernel's policy engine.
//
// The host does not decide what the policy engine is: the COMPOSITION ROOT fills the
// scope and the APPLICATION fills the family's own ports
// (`kernel/app/edict/ports.go`). What stays here is the binding — which kernel
// answers the policy family's questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) edictPorts() appedict.Ports { return s.scoped().EdictPorts }

// The REQUEST's policy ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// policy engine is the tenant's engine: the routed kernel is the one the
// request's services must read. The daemon's own ports answer for the root
// kernel.
func requestEdictPorts(ctx context.Context) appedict.Ports {
	return requestScope(ctx).EdictPorts
}
