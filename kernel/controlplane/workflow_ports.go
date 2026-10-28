// SPDX-License-Identifier: MIT
package controlplane

import (
	appworkflow "github.com/agezt/agezt/kernel/app/workflow"
)

// The workflow application's ports, bound to this daemon's kernel.
//
// The host does not decide what the workflows are read from: it hands over
// the kernel handle and the APPLICATION fills its own ports
// (`kernel/app/workflow/ports.go`). What stays here is the binding — which
// kernel answers the workflow's questions on this daemon.
func (s *Server) workflowPorts() appworkflow.Ports {
	return appworkflow.PortsFromKernel(s.k)
}
