// SPDX-License-Identifier: MIT

package standingapi

import "errors"

// Host is the port the standing-orders tool drives: the daemon's standing
// orders — the recurring instructions an operator leaves for the fleet.
//
// The tool is a PLUGIN, and the registry hands it the port — so the port
// lives in the contract layer, not in the tool's package (W3.39d). The row
// it names is the contract's own: `Order` is the same type the module keeps.
type Host interface {
	// AddStanding journals a new standing order.
	AddStanding(o Order) (Order, error)
	// RemoveStanding deletes a standing order by id; the bool says whether
	// it was there.
	RemoveStanding(id string) (bool, error)
	// StandingList returns every standing order.
	StandingList() []Order
}

// Lifecycle is the standing library's LIFECYCLE port: the writes every layer
// that owns standing orders needs.
//
// The tool's port (Host) names three of them; the layers that own standing
// orders — the standing application, the guardians' orders, the roster's
// teardown — need the other two as well. One port, one vocabulary: the
// standing library's operations live with the library (W3.76x), and the
// contract names them once.
//
// The port is the CONTRACT's because more than one party binds it: the
// applications are L4 and the tool is a PLUGIN (L6), and a plugin may reach
// the contract and nothing else.
type Lifecycle interface {
	Host
	// SetStandingEnabled pauses (false) or resumes (true) a standing order.
	SetStandingEnabled(id string, enabled bool) (Order, error)
	// UpdateStanding applies edits to an order's mutable fields; the bool
	// says whether the order was there.
	UpdateStanding(id string, mutate func(*Order)) (Order, bool, error)
}

// ErrNoLibrary is answered when the daemon holds no standing library at all.
//
// The standing library is the DAEMON's file (W3.50): the composition root
// opens it and hands the port over, and a daemon that was given none holds
// none — so the writes must SAY so rather than call into nothing.
//
// The row is the CONTRACT's (W3.76x): the library raises it (it is the
// library's absence), the applications raise it (they are the ones that
// notice), and a caller compares with errors.Is — so every side says the
// same thing.
var ErrNoLibrary = errors.New("standing: the standing library is not available on this daemon")
