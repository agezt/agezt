// SPDX-License-Identifier: MIT

package worldmodel

import (
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/event"
)

// busPublisher turns a bus into the CONTRACT's publisher port (W3.54).
//
// It lives in the test because the module no longer knows the bus: the graph
// takes the contract's port, and the layers above the module build the
// adapter over their own bus. The test needs a real bus (it asserts on the
// journal), so it builds the same adapter here.
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
