// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"

	appokr "github.com/agezt/agezt/kernel/app/okr"
)

// The REQUEST's objective ports.
//
// The host does not decide what the objectives are read from: the COMPOSITION ROOT
// fills the scope and the APPLICATION fills the family's own ports
// (`kernel/app/okr/ports.go`). What stays here is the reading — which scope
// answers the objective family's questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func requestOKRPorts(ctx context.Context) appokr.Ports {
	return requestScope(ctx).OkrPorts
}
