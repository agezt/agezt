// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"

	appeventbus "github.com/agezt/agezt/kernel/app/eventbus"
)

// The daemon's own bus ports: the root kernel's event bus.
//
// The host does not decide what the bus is: the COMPOSITION ROOT fills the scope
// and the APPLICATION fills the family's own ports
// (`kernel/app/eventbus/ports.go`). What stays here is the reading — which
// scope answers the bus family's questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) busPorts() appeventbus.Ports { return s.scoped().BusPorts }

// The REQUEST's bus ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's bus
// is the tenant's bus: the routed kernel is the one the request's publishers
// must write to. The daemon's own ports answer for the root kernel.
func requestBusPorts(ctx context.Context) appeventbus.Ports {
	return requestScope(ctx).BusPorts
}
