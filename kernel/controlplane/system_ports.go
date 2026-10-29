// SPDX-License-Identifier: MIT

package controlplane

import (
	"github.com/agezt/agezt/kernel/app/system"
)

// systemPorts is the daemon overview's ports, filled from the daemon's own
// kernel.
//
// The binding does not name the runtime: the handle reaches the application
// through the application's own port-filler, and the control plane does not
// spell the kernel's type to hand it over.
func (s *Server) systemPorts() system.Ports {
	return system.PortsFromKernel(s.k)
}
