// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"

	"github.com/agezt/agezt/kernel/app/binding"
	appworld "github.com/agezt/agezt/kernel/app/world"
)

// The REQUEST's world ports.
//
// The world model is the daemon's own graph, and every surface that reads it
// reads it on the REQUEST's path — so the routed kernel is the one that
// answers, and there is no daemon-level binding: a binding nobody calls is
// dead code.
//
// The host does not decide what the world is: it hands over the kernel handle
// and the APPLICATION fills its own ports (`kernel/app/world/ports.go`). What
// stays here is the binding — which kernel answers the world family's
// questions.
//
// The handle is never named here: this file does not import the runtime, so the
// adapter may hold the handle without naming the runtime package to hand it
// over.
func requestWorldPorts(ctx context.Context) appworld.Ports {
	return binding.Fill(ctx.Value(appHostKey{}).(appHost).kernel, appworld.PortsFromKernel)
}
