// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"

	appapprovals "github.com/agezt/agezt/kernel/app/approvals"
)

// The approvals family's ports.
//
// There are TWO of them, and they answer for two different kernels: the
// daemon's own and the routed one.
//
// The live approvals — the pending list and the decision — run on the
// REQUEST's path, so the ROUTED kernel is the one that answers: a tenant
// decides its own queue. The mission control's pending count and the plan
// runner's node spec are the DAEMON's own surfaces, so they ask the daemon's
// kernel.
//
// The host does not decide what the approvals are: the COMPOSITION ROOT fills the
// scope and the APPLICATION fills the family's own ports
// (`kernel/app/approvals/ports.go`). What stays here is the reading — which
// scope answers the approvals family's questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) approvalPorts() appapprovals.Ports {
	return s.scoped().ApprovalPorts
}

// requestApprovalPorts are the REQUEST's approval ports.
func requestApprovalPorts(ctx context.Context) appapprovals.Ports {
	return requestScope(ctx).ApprovalPorts
}
