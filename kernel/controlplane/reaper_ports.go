// SPDX-License-Identifier: MIT
package controlplane

import (
	appreaper "github.com/agezt/agezt/kernel/app/reaper"
)

// The reaper family's ports.
//
// The reaper's scan is the DAEMON's own — the read-only pass behind the
// operator's dead-agent report — and the two surfaces that ask for it (the
// reaper's own operations and the roster's status collection) are the
// daemon's own, so they ask the daemon's kernel. There is no request-level
// binding: a binding nobody calls is dead code.
//
// The host does not decide what the reaper is: the COMPOSITION ROOT fills the scope
// and the APPLICATION fills the family's own ports (`kernel/app/reaper/ports.go`). What
// stays here is the binding — which kernel answers the reaper family's
// questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) reaperPorts() appreaper.Ports {
	return s.scoped().ReaperScanPorts
}
