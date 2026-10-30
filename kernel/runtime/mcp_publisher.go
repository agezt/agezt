// SPDX-License-Identifier: MIT

package runtime

// Provenance: the MCP registry's PUBLISHER (W3.71). The extensions module
//             keeps the registry's lifecycle and publishes what happened to
//             a server on the HOST's bus — the host owns the bus, so the
//             host hands the module a way to ASK it.

import (
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
)

// mcpPublisher is the host's bus seen from the MCP registry: the module
// publishes one event and the host drops the reply.
type mcpPublisher struct{ b *bus.Bus }

// Publish emits one event on the host's bus, when the host has one.
func (p mcpPublisher) Publish(spec event.Spec) {
	if p.b == nil {
		return
	}
	_, _ = p.b.Publish(spec)
}
