// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"

	appokr "github.com/agezt/agezt/kernel/app/okr"
	"github.com/agezt/agezt/kernel/runtime"
)

// The objective application's ports, bound to a kernel.
//
// The host does not decide what the objectives are read from: it hands over
// the kernel handle and the APPLICATION fills its own ports
// (`kernel/app/okr/ports.go`). What stays here is the binding — which kernel
// answers the objective family's questions.
func okrPortsFor(k *runtime.Kernel) appokr.Ports { return appokr.PortsFromKernel(k) }

// The REQUEST's objective ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// objectives are the tenant's objectives: the routed kernel is the one the
// request's services must read.
func requestOKRPorts(ctx context.Context) appokr.Ports {
	return okrPortsFor(ctx.Value(appHostKey{}).(appHost).kernel)
}
