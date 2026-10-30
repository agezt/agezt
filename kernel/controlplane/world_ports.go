// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"

	appworld "github.com/agezt/agezt/kernel/app/world"
)

// The REQUEST's world ports.
//
// The world model is the daemon's own graph, and every surface that reads it
// reads it on the REQUEST's path — so the routed kernel is the one that
// answers, and there is no daemon-level binding: a binding nobody calls is
// dead code.
//
// The host does not decide what the world is: the COMPOSITION ROOT fills the scope
// and the APPLICATION fills the family's own ports (`kernel/app/world/ports.go`). What
// stays here is the binding — which kernel answers the world family's
// questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func requestWorldPorts(ctx context.Context) appworld.Ports {
	return requestScope(ctx).WorldPorts
}
