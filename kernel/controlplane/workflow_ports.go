// SPDX-License-Identifier: MIT
package controlplane

import (
	appworkflow "github.com/agezt/agezt/kernel/app/workflow"
)

// The workflow application's ports, bound to this daemon's kernel.
//
// The host does not decide what the workflows are read from: the COMPOSITION ROOT
// fills the scope and the APPLICATION fills the family's own ports
// (`kernel/app/workflow/ports.go`). What stays here is the reading — which
// scope answers the workflow's questions on this daemon.
func (s *Server) workflowPorts() appworkflow.Ports {
	return s.scoped().WorkflowEnginePorts
}
