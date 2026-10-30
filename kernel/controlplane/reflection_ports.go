// SPDX-License-Identifier: MIT
package controlplane

import (
	appreflection "github.com/agezt/agezt/kernel/app/reflection"
)

// The reflection family's ports.
//
// The reflection engine is the DAEMON's own — the composition root always
// builds one — and the surface that triggers and reads a pass is the daemon's
// own, so it asks the daemon's kernel. There is no request-level binding: a
// binding nobody calls is dead code.
//
// The host does not decide what the engine is: the COMPOSITION ROOT fills the scope
// and the APPLICATION fills the family's own ports
// (`kernel/app/reflection/ports.go`). What stays here is the reading — which
// scope answers the reflection family's questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) reflectionPorts() appreflection.Ports {
	return s.scoped().ReflectionPorts
}
