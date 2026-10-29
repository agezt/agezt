// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"

	appcatalog "github.com/agezt/agezt/kernel/app/catalog"
)

// The daemon's own catalog ports: the root kernel's catalog directory.
//
// The host does not decide what the catalog is: it hands over the kernel handle
// and the APPLICATION fills its own ports (`kernel/app/catalog/ports.go`). What
// stays here is the binding — which kernel answers the catalog family's
// questions.
//
// The handle is never named here: this file does not import the runtime, so the
// adapter may hold the handle without naming the runtime package to hand it
// over.
func (s *Server) catalogPorts() appcatalog.Ports { return appcatalog.PortsFromKernel(s.k) }

// The REQUEST's catalog ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// catalog is the tenant's catalog: the routed kernel is the one the request's
// services must read. The daemon's own ports answer for the root kernel.
func requestCatalogPorts(ctx context.Context) appcatalog.Ports {
	return appcatalog.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
