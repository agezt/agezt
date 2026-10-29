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
// The host does not decide what the approvals are: it hands over the kernel
// handle and the APPLICATION fills its own ports
// (`kernel/app/approvals/ports.go`). What stays here is the binding — which
// kernel answers the approvals family's questions.
//
// The handle is never named here: this file does not import the runtime, so the
// adapter may hold the handle without naming the runtime package to hand it
// over.
func (s *Server) approvalPorts() appapprovals.Ports {
	return appapprovals.PortsFromKernel(s.k)
}

// requestApprovalPorts are the REQUEST's approval ports.
func requestApprovalPorts(ctx context.Context) appapprovals.Ports {
	return appapprovals.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
