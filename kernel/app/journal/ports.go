// SPDX-License-Identifier: MIT
package journal

import (
	journalstore "github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/runtime"
)

// The journal application's ports, filled from the kernel handle.
//
// The APPLICATION owns this: it knows which capabilities the journal family
// needs and how each of them answers. The host hands over the handle and
// nothing else — it does not decide what the journal is read from.
//
// The journal is the one capability EVERY family reads, and the families
// already narrow it: each consumer declares its OWN interface over the part of
// the journal it uses — the roster's range, the channel inbox's append, the
// pulse stream's tail. The narrow views are the consumers' business, not the
// family's, so the port carries the journal itself and every consumer cuts it
// down to what it needs.
//
// The journal is PLATFORM (layer 2), so its rows may be named by the
// application layer and by the adapter — the row is not the boundary. The
// boundary is the kernel's VERB: the adapter must not reach for the kernel's
// journal, it must ask this family.

// Ports are the journal application's capabilities over the event journal.
type Ports struct {
	// Events is the journal — the event log every family reads.
	//
	// The field answers to the FAMILY's name, not to the kernel's verb: the
	// kernel's verb is "Journal", and a port field that answers to a verb's
	// name is a verb the adapter may name.
	Events *journalstore.Journal
}

// PortsFromKernel fills the journal application's ports from the kernel
// handle.
func PortsFromKernel(k *runtime.Kernel) Ports {
	return Ports{Events: k.Journal()}
}
