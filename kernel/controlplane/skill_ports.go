// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"

	"github.com/agezt/agezt/kernel/app/binding"
	appskill "github.com/agezt/agezt/kernel/app/skill"
)

// The daemon's own skill ports: the root kernel's skill library.
//
// The host does not decide what the skill library is: it hands over the kernel
// handle and the APPLICATION fills its own ports
// (`kernel/app/skill/ports.go`). What stays here is the binding — which kernel
// answers the skill family's questions.
//
// The handle is never named here: this file does not import the runtime, so
// the adapter may hold the handle without naming the runtime package to hand
// it over.
func (s *Server) skillPorts() appskill.Ports { return binding.Fill(s.k, appskill.PortsFromKernel) }

// The REQUEST's skill ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// skill library is the tenant's library: the routed kernel is the one the
// request's services must read. The daemon's own ports answer for the root
// kernel.
func requestSkillPorts(ctx context.Context) appskill.Ports {
	return binding.Fill(ctx.Value(appHostKey{}).(appHost).kernel, appskill.PortsFromKernel)
}
