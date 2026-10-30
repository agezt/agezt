// SPDX-License-Identifier: MIT

package controlplane

import (
	appartifacts "github.com/agezt/agezt/kernel/app/artifacts"
)

// The daemon's own artifact ports: the root kernel's artifact store and its
// index.
//
// The host does not decide what the artifact store is: the COMPOSITION ROOT fills the
// scope and the APPLICATION fills the family's own ports
// (`kernel/app/artifacts/ports.go`). What stays here is the reading — which
// scope answers the artifacts family's questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) artifactPorts() appartifacts.Ports {
	return s.scoped().ArtifactPorts
}
