// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"

	apppersona "github.com/agezt/agezt/kernel/app/persona"
)

// The persona family's ports.
//
// There are TWO of them, and they answer for two different kernels: the
// daemon's own and the routed one.
//
// The persona SURFACE — the owner's chat defaults, the identity the owner edits
// — is PRIMARY only, so it is the daemon's own: `s.personaPorts()`.
//
// The config surface REPORTS the identity — the model and whether a prompt is
// set — and it runs on the REQUEST's path, so it asks the ROUTED kernel:
// `requestPersonaPorts(ctx)`.
//
// The host does not decide what the identity is: it hands over the kernel
// handle and the APPLICATION fills its own ports
// (`kernel/app/persona/ports.go`). What stays here is the binding — which
// kernel answers the persona family's questions.
//
// The handle is never named here: this file does not import the runtime, so the
// adapter may hold the handle without naming the runtime package to hand it
// over.
func (s *Server) personaPorts() apppersona.Ports {
	return apppersona.PortsFromKernel(s.k)
}

// requestPersonaPorts are the REQUEST's persona ports.
func requestPersonaPorts(ctx context.Context) apppersona.Ports {
	return apppersona.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
