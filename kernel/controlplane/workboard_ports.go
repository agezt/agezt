// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"

	appworkboard "github.com/agezt/agezt/kernel/app/workboard"
)

// The daemon's own workboard ports: the root kernel's board.
//
// The host does not decide what the board is read from: the COMPOSITION ROOT fills the
// scope and the APPLICATION fills the family's own ports
// (`kernel/app/workboard/ports.go`). What stays here is the reading — which
// scope answers the workboard family's questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) workboardPorts() appworkboard.Ports {
	return s.scoped().WorkboardPorts
}

// The REQUEST's workboard ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// board is the tenant's board: the routed kernel is the one the request's
// services must read.
func requestWorkboardPorts(ctx context.Context) appworkboard.Ports {
	return requestScope(ctx).WorkboardPorts
}
