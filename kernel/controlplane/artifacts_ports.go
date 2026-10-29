// SPDX-License-Identifier: MIT

package controlplane

import (
	appartifacts "github.com/agezt/agezt/kernel/app/artifacts"
)

// The daemon's own artifact ports: the root kernel's artifact store and its
// index.
//
// The host does not decide what the artifact store is: it hands over the kernel
// handle and the APPLICATION fills its own ports
// (`kernel/app/artifacts/ports.go`). What stays here is the binding — which
// kernel answers the artifacts family's questions.
//
// The handle is never named here: this file does not import the runtime, so the
// adapter may hold the handle without naming the runtime package to hand it
// over.
func (s *Server) artifactPorts() appartifacts.Ports { return appartifacts.PortsFromKernel(s.k) }
