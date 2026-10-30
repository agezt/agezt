// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"

	appjournal "github.com/agezt/agezt/kernel/app/journal"
)

// The daemon's own journal ports: the root kernel's event log.
//
// The host does not decide what the journal is read from: the COMPOSITION ROOT fills the
// scope and the APPLICATION fills the family's own ports
// (`kernel/app/journal/ports.go`). What stays here is the reading — which
// scope answers the journal family's questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func (s *Server) journalPorts() appjournal.Ports {
	return s.scoped().JournalPorts
}

// The REQUEST's journal ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// journal is the tenant's journal: the routed kernel is the one the request's
// services must read. The daemon's own ports answer for the root kernel.
func requestJournalPorts(ctx context.Context) appjournal.Ports {
	return requestScope(ctx).JournalPorts
}
