// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"

	"github.com/agezt/agezt/kernel/app/binding"
	appjournal "github.com/agezt/agezt/kernel/app/journal"
)

// The daemon's own journal ports: the root kernel's event log.
//
// The host does not decide what the journal is read from: it hands over the
// kernel handle and the APPLICATION fills its own ports
// (`kernel/app/journal/ports.go`). What stays here is the binding — which
// kernel answers the journal family's questions.
//
// The handle is never named here: this file does not import the runtime, so
// the adapter may hold the handle without naming the runtime package to hand
// it over.
func (s *Server) journalPorts() appjournal.Ports {
	return binding.Fill(s.k, appjournal.PortsFromKernel)
}

// The REQUEST's journal ports.
//
// A tenant's operations run on the tenant's own kernel, and the tenant's
// journal is the tenant's journal: the routed kernel is the one the request's
// services must read. The daemon's own ports answer for the root kernel.
func requestJournalPorts(ctx context.Context) appjournal.Ports {
	return binding.Fill(ctx.Value(appHostKey{}).(appHost).kernel, appjournal.PortsFromKernel)
}
