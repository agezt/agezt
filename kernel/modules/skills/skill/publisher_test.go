// SPDX-License-Identifier: MIT

package skill

// Provenance: the test-side bus adapter for the contract's publisher port.
//             The Forge journals through a PORT (W3.55), and the port answers
//             with the event's id; the bus answers with the durable envelope.
//             The tests that assert on the journal need both, so the bus is
//             wrapped here — the same shape the kernel's own adapter has.

import (
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/event"
)

// busPublisher publishes one durable event on a test bus and answers its id.
type busPublisher struct{ b *bus.Bus }

func (p busPublisher) Publish(spec eventapi.Spec) (string, error) {
	if p.b == nil {
		return "", nil
	}
	ev, err := p.b.Publish(event.Spec(spec))
	if err != nil {
		return "", err
	}
	return ev.ID, nil
}
