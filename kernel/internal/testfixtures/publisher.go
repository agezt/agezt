// SPDX-License-Identifier: MIT

package testfixtures

// Provenance: the shared test adapter for the CONTRACT's publisher port
//             (W3.57). The memory module names no bus — it holds the
//             contract's publisher port — so the tests that assert on the
//             JOURNAL need an adapter of their own.
//
// The port answers with the id of the event it wrote: that is what a record
// keeps as its provenance, and the journaled envelope is what the bus
// answers with.

import (
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/event"
)

// BusPublisher adapts the journal bus to the contract's publisher port.
type BusPublisher struct{ B *bus.Bus }

// Publish writes one durable event and answers its id.
func (p BusPublisher) Publish(spec eventapi.Spec) (string, error) {
	if p.B == nil {
		return "", nil
	}
	ev, err := p.B.Publish(event.Spec(spec))
	if err != nil {
		return "", err
	}
	return ev.ID, nil
}
