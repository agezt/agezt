// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"

	appprovider "github.com/agezt/agezt/kernel/app/provider"
)

// The daemon's own provider ports: the root kernel's live provider.
//
// The host does not decide what the provider is: it hands over the kernel
// handle and the APPLICATION fills its own ports
// (`kernel/app/provider/ports.go`). What stays here is the binding — which
// kernel answers the provider family's questions.
//
// The handle is never named here: this file does not import the runtime, so
// the adapter may hold the handle without naming the runtime package to hand
// it over.
func (s *Server) providerPorts() appprovider.Ports { return appprovider.PortsFromKernel(s.k) }

// The REQUEST's provider ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// provider is the tenant's provider: the routed kernel is the one the
// request's services must read. The daemon's own ports answer for the root
// kernel.
func requestProviderPorts(ctx context.Context) appprovider.Ports {
	return appprovider.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
