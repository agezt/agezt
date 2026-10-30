// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"

	appskill "github.com/agezt/agezt/kernel/app/skill"
)

// The daemon's own skill ports: the root kernel's skill library.
//
// The host does not decide what the skill library is: the COMPOSITION ROOT fills the
// scope and the APPLICATION fills the family's own ports
// (`kernel/app/skill/ports.go`). What stays here is the binding — which kernel
// answers the skill family's questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) skillPorts() appskill.Ports { return s.scoped().SkillPorts }

// The REQUEST's skill ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// skill library is the tenant's library: the routed kernel is the one the
// request's services must read. The daemon's own ports answer for the root
// kernel.
func requestSkillPorts(ctx context.Context) appskill.Ports {
	return requestScope(ctx).SkillPorts
}
