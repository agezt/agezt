// SPDX-License-Identifier: MIT
package controlplane

import (
	applifecycle "github.com/agezt/agezt/kernel/app/lifecycle"
)

// The daemon lifecycle family's ports.
//
// The daemon's lifecycle capability — drain the runs in flight and halt — is
// the DAEMON's own, and the flow that asks for it (the update) asks the
// daemon's kernel. There is no request-level binding: a binding nobody calls
// is dead code.
//
// The host does not decide how a daemon is drained: the COMPOSITION ROOT fills the
// scope and the APPLICATION fills the family's own ports
// (`kernel/app/lifecycle/ports.go`). What stays here is the reading — which
// scope answers the lifecycle family's questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) lifecyclePorts() applifecycle.Ports {
	return s.scoped().LifecyclePorts
}
