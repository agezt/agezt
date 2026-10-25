// SPDX-License-Identifier: MIT

package runtime

import (
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/event"
)

// worldPublisher publishes one durable event on the kernel's bus.
//
// NOTHING is translated on the way in: the spec the contract's port takes is
// the same row the bus takes (the platform's event package keeps it as an
// alias of the contract's), so what the graph publishes is exactly what the
// bus writes. The port answers with the event's ID — that is the graph's
// provenance (SourceEvent), and `agt why` reads it.
//
// A nil bus must stay a NIL PORT: a port that wraps a nil implementation is
// not nil, so the graph would publish into nothing instead of skipping the
// event. The check is on the concrete type on purpose — a nil *bus.Bus handed
// to an interface parameter is already a non-nil interface.
type worldPublisher struct{ b *bus.Bus }

func (p worldPublisher) Publish(spec eventapi.Spec) (string, error) {
	if p.b == nil {
		return "", nil
	}
	ev, err := p.b.Publish(event.Spec(spec))
	if err != nil {
		return "", err
	}
	return ev.ID, nil
}
