// SPDX-License-Identifier: MIT

// Package journalapi is the contract layer's home for the journal's READ
// side: the projection of a durable event a reader gets to see, and the port
// through which recent events are read.
//
// The rows live here because the tools that read the daemon's own history
// are PLUGINS, and a plugin may reach the contract and the platform layers
// and nothing else. The implementation that keeps them — the append-only
// journal with its hash chain — stays in the platform (kernel/journal); the
// platform keeps the names its own callers use as aliases.
//
// The row is a PROJECTION, not the durable event: the durable envelope
// carries the hash chain and the canonical bytes, and those are
// implementation — the contract layer may not import a third-party hash
// (archcheck's principle P4). A reader that folds the daemon's history into
// runs needs the kind, the correlation, the timestamp and the payload, and
// nothing else, so that is exactly what the row carries.
//
// The rules of the road:
//
//   - Rows, constants and pure functions only. No file, no clock.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package journalapi

import (
	"encoding/json"

	"github.com/agezt/agezt/kernel/contract/eventapi"
)

// Record is one durable event, as a reader sees it.
//
// The four fields are the whole of what a reader needs to fold the daemon's
// history: WHICH kind of thing happened, WHICH run it belongs to, WHEN it
// happened, and WHAT it carried. The durable envelope's own fields (the
// hash chain, the sequence, the subject) belong to the journal, not to the
// reader.
type Record struct {
	// Kind is the event's discriminator — the same row the platform's
	// event registry owns.
	Kind eventapi.Kind
	// CorrelationID ties the event to the run it belongs to.
	CorrelationID string
	// TSUnixMS is when the event happened (wall clock, milliseconds).
	TSUnixMS int64
	// Payload is the event's JSON body, verbatim. Nil when the event
	// carried none.
	Payload json.RawMessage
}

// Reader is the port through which recent events are read.
//
// It is the read side of the journal: the tool that recalls the daemon's
// past runs reads through it, and the daemon supplies the live journal's
// implementation (the composition root adapts it, because the journal is a
// platform package the tool may not name).
type Reader interface {
	// Tail returns the last n events, oldest first. An error means the
	// history could not be read.
	Tail(n int) ([]Record, error)
}

// Scanner is the port through which the WHOLE history is walked.
//
// The Reader answers "what happened most recently"; the Scanner answers "what
// happened at all". A maintenance task that reports the journal's pressure
// needs every event's timestamp and the chain's head, and nothing else — so
// this is the second read port over the same rows, not a second copy of them.
//
// The scan visits the events oldest first. The visit function's error stops
// the walk and is returned.
type Scanner interface {
	// Scan visits every durable event, oldest first.
	Scan(visit func(Record) error) error
	// Head answers the journal's head: the sequence number of the last
	// event and its hash. Both are zero when the journal is empty.
	Head() (seq int64, hash string)
}
