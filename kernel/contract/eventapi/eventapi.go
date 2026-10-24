// SPDX-License-Identifier: MIT

// Package eventapi is the contract layer's home for the event rows: the kind
// discriminator, the caller-provided portion of an event, and the port
// through which a durable kernel event is published.
//
// The rows live here because the tools that publish kernel events are
// PLUGINS, and a plugin may reach the contract and the platform layers and
// nothing else. The implementation that keeps them — the durable envelope
// with its hash chain, the journal that appends it, the bus that fans it out
// — stays where it is (kernel/event, kernel/journal, kernel/bus); those
// packages keep the names their own callers use as aliases, so there is one
// row and one spelling of it.
//
// The SPLIT is deliberate and it is the only one that matters here: the
// ENVELOPE (the hash chain, the canonical bytes, the BLAKE3 digest) is
// implementation and stays in the platform — the contract layer may not
// import a third-party hash (archcheck's principle P4). The ROWS a caller
// fills in — what kind, who, which correlation, what payload — are pure and
// travel.
//
// The rules of the road:
//
//   - Rows, constants and pure functions only. No clock, no hash, no file.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package eventapi

// Kind is the canonical event-kind discriminator (e.g. "agent.spawned").
// Kinds are pinned by the wire contract (.project/agezt-contract.jsonc) and
// grow append-only as new layers land (DECISIONS B0b). Never renumber or
// rename an existing kind; only add new ones.
//
// The TYPE lives here so the contract's rows can name it; the VOCABULARY
// (the Kind* constants) stays in the platform package that owns the event
// registry (kernel/event), which keeps its own guard test over it.
type Kind string

// Spec is the caller-provided portion of an event. The kernel (specifically
// the journal) supplies ID, Seq, TSUnixMS, PrevHash and computes Hash.
//
// The row is the same row the bus and the journal have always taken — the
// platform's package keeps it as an ALIAS of this one, so a spec that
// crosses the contract's port and a spec the platform builds are the same
// type, not two shapes that have to be kept in step by hand.
type Spec struct {
	Subject       string            // hierarchical routing key (e.g. "agent.01H...tool")
	Kind          Kind              // event-kind discriminator
	Actor         string            // who emitted (agent ID, plugin ID, "kernel", ...)
	CorrelationID string            // ties a multi-event causal chain (optional)
	CausationID   string            // the event that caused this one (optional)
	Payload       any               // JSON-marshalable; nil → omitted
	Tags          map[string]string // free-form labels (optional)
}

// Publisher is the port through which a durable kernel event is published.
//
// The port is NARROWER than the bus's own method on purpose: the bus returns
// the durable event it wrote, and that event carries the hash chain — a row
// the contract layer may not name. A publisher that only needs to know
// whether the write succeeded asks exactly that.
type Publisher interface {
	// Publish writes the event durably and fans it out to the live
	// subscribers. An error means the event did not land.
	Publish(spec Spec) error
}
