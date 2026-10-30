// SPDX-License-Identifier: MIT

package controlplane

import (
	approster "github.com/agezt/agezt/kernel/app/roster"
)

// The roster's profile ports, bound to this daemon's kernel.
//
// The host does not decide what the roster reads: the COMPOSITION ROOT fills the
// scope and the APPLICATION fills the family's own ports
// (`kernel/app/roster/ports.go`). What stays here is the reading — which
// scope answers the roster's questions on this daemon.
func (s *Server) rosterPorts() approster.ProfilePorts {
	return s.scoped().RosterProfilePorts
}
