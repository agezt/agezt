// SPDX-License-Identifier: MIT

package standingapi

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
