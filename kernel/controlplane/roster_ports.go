// SPDX-License-Identifier: MIT

package controlplane

import (
	approster "github.com/agezt/agezt/kernel/app/roster"
)

// The roster's profile ports, bound to this daemon's kernel.
//
// The host does not decide what the roster reads: it hands over the kernel
// handle and the APPLICATION fills its own ports
// (`kernel/app/roster/ports.go`). What stays here is the binding — which
// kernel answers the roster's questions on this daemon.
func (s *Server) rosterPorts() approster.ProfilePorts {
	return approster.ProfilePortsFromKernel(s.k)
}
