// SPDX-License-Identifier: MIT

package standingapi

import "errors"

// ErrNotFound is returned for operations on an unknown order id.
//
// It lives in the contract layer because more than one layer has to NAME it:
// the store that answers it, the kernel that turns it into "not found" for
// the edit path, and the callers that ask "did that order exist?" — the
// control plane's roster teardown among them. A module may not be named by
// the layers above it, so the error the layers above it compare against must
// not live in the module (W3.50).
var ErrNotFound = errors.New("standing: order not found")

// Store is the port the daemon's standing-order library implements: the
// durable set of standing orders, one JSON file rewritten atomically on
// change.
//
// The port lives in the contract layer because three layers have to speak it
// and none of them may name the module's implementation:
//
//   - the kernel holds it (cfg.Standing) and drives the lifecycle through it
//     — adding an order journals standing.created, pausing one journals
//     standing.updated;
//   - the app layer's service holds it as its READER (List/Get) and reaches
//     the kernel's writer for the mutations;
//   - the control plane reads it through the kernel's accessor.
//
// The daemon opens the file and hands the kernel this port; a host that was
// given none holds a nil port and reports that the library is unavailable
// rather than calling into nothing.
//
// The port is the module's whole public surface — seven verbs, no more. A
// wider surface would let a caller reach past the library's own rules (the
// store validates, assigns ids and protects identity fields); a narrower one
// would force a second shape to be kept in step with this one.
type Store interface {
	// Add validates and persists a new order, assigning its id and
	// timestamps. The caller-supplied id/enabled/timestamps are ignored.
	Add(o Order) (Order, error)
	// SetEnabled pauses (false) or resumes (true) an order and returns its
	// new state. ErrNotFound for an unknown id.
	SetEnabled(id string, enabled bool) (Order, error)
	// Update applies edits to an order's mutable fields via mutate,
	// re-validates and persists. Identity and lifecycle fields the caller
	// must not edit are preserved. ErrNotFound for an unknown id.
	Update(id string, mutate func(*Order)) (Order, error)
	// Remove deletes an order; the bool says whether it was there.
	Remove(id string) (bool, error)
	// Get returns one order by id; the bool says whether it was there.
	Get(id string) (Order, bool)
	// List returns every order, in a deterministic order.
	List() []Order
	// Count returns the number of orders.
	Count() int
}
