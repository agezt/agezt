// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"

	appbasedir "github.com/agezt/agezt/kernel/app/basedir"
)

// The daemon's own data root: the root kernel's base directory.
//
// The host does not decide where the daemon keeps its files: the COMPOSITION ROOT fills the
// scope and the APPLICATION fills the family's own ports
// (`kernel/app/basedir/ports.go`). What stays here is the binding — which
// kernel's root answers.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) baseDirPorts() appbasedir.Ports {
	return s.scoped().BasedirPorts
}

// The REQUEST's data root.
//
// A tenant's operations run on the tenant's own kernel, and a tenant's kernel
// keeps its files under the tenant's OWN root — the tenant registry opens one
// base directory per tenant. The daemon's own ports answer for the root
// kernel.
func requestBaseDirPorts(ctx context.Context) appbasedir.Ports {
	return requestScope(ctx).BasedirPorts
}
