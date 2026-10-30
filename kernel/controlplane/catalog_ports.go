// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"

	appcatalog "github.com/agezt/agezt/kernel/app/catalog"
)

// The daemon's own catalog ports: the root kernel's catalog directory.
//
// The host does not decide what the catalog is: the COMPOSITION ROOT fills the scope
// and the APPLICATION fills the family's own ports (`kernel/app/catalog/ports.go`). What
// stays here is the binding — which kernel answers the catalog family's
// questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) catalogPorts() appcatalog.Ports {
	return s.scoped().CatalogPorts
}

// The REQUEST's catalog ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// catalog is the tenant's catalog: the routed kernel is the one the request's
// services must read. The daemon's own ports answer for the root kernel.
func requestCatalogPorts(ctx context.Context) appcatalog.Ports {
	return requestScope(ctx).CatalogPorts
}
