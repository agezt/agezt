// SPDX-License-Identifier: MIT

package reflect

import (
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/event"
)

// busPublisher turns a bus into the CONTRACT's publisher port (W3.54).
//
// The graph takes the contract's port; the layers above the module build the
// adapter over their own bus. The test needs a real bus, so it builds the same
// adapter here.
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
