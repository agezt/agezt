// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"

	"github.com/agezt/agezt/kernel/app/binding"
	apptaste "github.com/agezt/agezt/kernel/app/taste"
)

// The REQUEST's taste ports.
//
// The taste library is the daemon's own file, and every surface that reads it
// reads it on the REQUEST's path — so the routed kernel is the one that
// answers, and there is no daemon-level binding: a binding nobody calls is
// dead code.
//
// The host does not decide what the taste is: it hands over the kernel handle
// and the APPLICATION fills its own ports (`kernel/app/taste/ports.go`). What
// stays here is the binding — which kernel answers the taste family's
// questions.
//
// The handle is never named here: this file does not import the runtime, so the
// adapter may hold the handle without naming the runtime package to hand it
// over.
func requestTastePorts(ctx context.Context) apptaste.Ports {
	return binding.Fill(ctx.Value(appHostKey{}).(appHost).kernel, apptaste.PortsFromKernel)
}
