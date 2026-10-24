// SPDX-License-Identifier: MIT

package controlplane

import (
	"strings"

	"github.com/agezt/agezt/kernel/contract/seatapi"
)

// seatValidator is the seat-validity check the workboard operations use.
//
// The host holds the contract's port, and the port may be missing: the seat
// catalog is the daemon's file, and a host that was given no catalog holds no
// port at all. A missing catalog must not become a panic on a nil interface.
//
// The missing port answers exactly what the port would answer for the case
// that needs no catalog: an empty id means "no seat" and stays valid (a task
// that asks for no seat is not asking for an unknown one), and a NAMED seat
// cannot be confirmed without a catalog, so it is refused and the operation
// reports "unknown execution seat".
//
// A catalog that IS there is asked, not assumed: the port's own answer is
// what decides.
func seatValidator(store seatapi.Store) func(string) bool {
	if store == nil {
		return func(id string) bool { return strings.TrimSpace(id) == "" }
	}
	return store.Valid
}
