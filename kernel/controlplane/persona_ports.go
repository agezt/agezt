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
// The host does not decide what the identity is: the COMPOSITION ROOT fills the
// scope and the APPLICATION fills the family's own ports
// (`kernel/app/persona/ports.go`). What stays here is the reading — which
// scope answers the persona family's questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) personaPorts() apppersona.Ports {
	return s.scoped().PersonaPorts
}

// requestPersonaPorts are the REQUEST's persona ports.
func requestPersonaPorts(ctx context.Context) apppersona.Ports {
	return requestScope(ctx).PersonaPorts
}
