// SPDX-License-Identifier: MIT

package memory

// Provenance: the module's own test adapters for the CONTRACT's ports
//             (W3.57). The module names no bus and no other module, so the
//             tests bring their own: a bus-shaped publisher for the
//             journaling assertions, and the structured-output path for the
//             distillation assertions.

import (
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/event"
)

// testPublisher adapts the journal bus to the contract's publisher port.
//
// The port answers with the id of the event it wrote — that is what a record
// keeps as its provenance — and the bus answers with the durable envelope.
type testPublisher struct{ b *bus.Bus }

func (p testPublisher) Publish(spec eventapi.Spec) (string, error) {
	if p.b == nil {
		return "", nil
	}
	ev, err := p.b.Publish(event.Spec(spec))
	if err != nil {
		return "", err
	}
	return ev.ID, nil
}

// testGenerator is the structured-output path the host injects in production.
// The module cannot name it — it lives in another module — so the tests hand
// it in the same way the daemon does.
var testGenerator memoryapi.Generator = agent.GenerateObject
