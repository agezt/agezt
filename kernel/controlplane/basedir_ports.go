// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"

	appbasedir "github.com/agezt/agezt/kernel/app/basedir"
	"github.com/agezt/agezt/kernel/app/binding"
)

// The daemon's own data root: the root kernel's base directory.
//
// The host does not decide where the daemon keeps its files: it hands over the
// kernel handle and the APPLICATION fills its own ports
// (`kernel/app/basedir/ports.go`). What stays here is the binding — which
// kernel's root answers.
//
// The handle is never named here: this file does not import the runtime, so
// the adapter may hold the handle without naming the runtime package to hand
// it over.
func (s *Server) baseDirPorts() appbasedir.Ports {
	return binding.Fill(s.k, appbasedir.PortsFromKernel)
}

// The REQUEST's data root.
//
// A tenant's operations run on the tenant's own kernel, and a tenant's kernel
// keeps its files under the tenant's OWN root — the tenant registry opens one
// base directory per tenant. The daemon's own ports answer for the root
// kernel.
func requestBaseDirPorts(ctx context.Context) appbasedir.Ports {
	return binding.Fill(ctx.Value(appHostKey{}).(appHost).kernel, appbasedir.PortsFromKernel)
}
