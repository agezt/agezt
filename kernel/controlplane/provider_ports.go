// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"

	appprovider "github.com/agezt/agezt/kernel/app/provider"
)

// The daemon's own provider ports: the root kernel's live provider.
//
// The host does not decide what the provider is: the COMPOSITION ROOT fills the
// scope and the APPLICATION fills the family's own ports
// (`kernel/app/provider/ports.go`). What stays here is the reading — which
// scope answers the provider family's questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) providerPorts() appprovider.Ports {
	return s.scoped().ProviderPorts
}

// The REQUEST's provider ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// provider is the tenant's provider: the routed kernel is the one the
// request's services must read. The daemon's own ports answer for the root
// kernel.
func requestProviderPorts(ctx context.Context) appprovider.Ports {
	return requestScope(ctx).ProviderPorts
}
