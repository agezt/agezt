// SPDX-License-Identifier: MIT
package controlplane

import (
	appconfigcenter "github.com/agezt/agezt/kernel/app/configcenter"
)

// The config center family's ports, bound to this daemon's kernel.
//
// The host does not decide what the config center is read from: the COMPOSITION ROOT
// fills the scope and the APPLICATION fills the family's own ports
// (`kernel/app/configcenter/ports.go`). What stays here is the reading — which
// scope answers the config center family's questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) configCenterPorts() appconfigcenter.Ports {
	return s.scoped().ConfigCenterPorts
}
