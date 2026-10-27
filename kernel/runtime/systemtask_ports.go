// SPDX-License-Identifier: MIT

package runtime

import (
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/contract/journalapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// SystemTaskPorts hands the cadence module the capabilities its system tasks
// run on.
//
// The tasks used to take the kernel itself, which made the cadence module
// depend on the runs module. They still need the daemon's capabilities; what
// changes is HOW they ask: through the contract's ports, so the module names
// the contract and never the kernel.
//
// The conversion happens HERE, where the values are still concrete. A nil
// *bus.Bus or a nil *journal.Journal handed to an interface parameter is a
// NON-NIL interface, and the holder would then hold a capability that is not
// there — so the check is on the concrete type, and a capability the daemon
// does not have is a NIL PORT. A nil port is a refusal, not an empty answer.
func (k *Kernel) SystemTaskPorts() cadenceapi.SystemTaskPorts {
	ports := cadenceapi.SystemTaskPorts{
		Memory:         k.Memory(),
		DistillProfile: k.DistillProfile,
		RosterList:     k.RosterList,
		CatalogSyncer:  k.CatalogSyncer(),
		CatalogStore:   k.CatalogStore(),
		Reload:         k.Reload,
	}
	if k.bus != nil {
		ports.Publish = systemTaskPublisher{b: k.bus}
	}
	if k.journal != nil {
		ports.ScanJournal = systemTaskScanner{j: k.journal}
	}
	if idx := k.ArtifactIndex(); idx != nil {
		// The collect verb and NOT the ten-verb host port: two of the
		// index's verbs delete bytes and one of them switches off the
		// journal's ref guard, and a task that only collects has no business
		// holding them.
		ports.CollectArtifacts = func(olderThanMs int64) (int, int64) {
			return idx.Collect(olderThanMs)
		}
	}
	return ports
}

// systemTaskPublisher publishes one durable event on the kernel's bus.
//
// NOTHING is translated on the way in: the spec the contract's port takes is
// the same row the bus takes (the platform's event package keeps it as an
// alias of the contract's), so what the task publishes is exactly what the
// bus writes.
type systemTaskPublisher struct{ b *bus.Bus }

// Publish writes the event durably and fans it out to the live subscribers.
func (p systemTaskPublisher) Publish(spec eventapi.Spec) error {
	_, err := p.b.Publish(spec)
	return err
}

// systemTaskScanner walks the daemon's durable history through the contract's
// scan port.
//
// It is the ADAPTER the durable envelope needs: the journal hands back the
// durable event (with its hash chain and its sequence), and the contract's
// port hands back the read PROJECTION — the kind, the correlation, the
// timestamp and the payload, which is everything a reader folds.
type systemTaskScanner struct{ j *journal.Journal }

// Scan visits every durable event, oldest first.
func (s systemTaskScanner) Scan(visit func(journalapi.Record) error) error {
	return s.j.Range(func(e *event.Event) error {
		if e == nil {
			return nil
		}
		return visit(journalapi.Record{
			Kind:          e.Kind,
			CorrelationID: e.CorrelationID,
			TSUnixMS:      e.TSUnixMS,
			Payload:       e.Payload,
		})
	})
}

// Head answers the chain's head: the last event's sequence and hash.
func (s systemTaskScanner) Head() (int64, string) { return s.j.Head() }
