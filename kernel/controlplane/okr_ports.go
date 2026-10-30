// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"

	"github.com/agezt/agezt/kernel/app/binding"
	appokr "github.com/agezt/agezt/kernel/app/okr"
)

// The REQUEST's objective ports.
//
// The host does not decide what the objectives are read from: it hands over
// the kernel handle and the APPLICATION fills its own ports
// (`kernel/app/okr/ports.go`). What stays here is the binding — which kernel
// answers the objective family's questions.
//
// The handle is never named here: this file does not import the runtime, so
// the adapter may hold the handle without naming the runtime package to hand
// it over.
func requestOKRPorts(ctx context.Context) appokr.Ports {
	return binding.Fill(ctx.Value(appHostKey{}).(appHost).kernel, appokr.PortsFromKernel)
}
