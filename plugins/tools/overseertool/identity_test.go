// SPDX-License-Identifier: MIT

package overseertool

import (
	"github.com/agezt/agezt/kernel/contract/overseerapi"
)

// The port the tool steers the fleet through IS the contract's port — the
// contract layer is where a plugin's port has to live (a plugin may reach the
// contract and the platform layers and nothing else), and the app layer is
// where the implementation that answers it lives.
//
// The assertions are FUNCTION types on purpose: an assignment would also
// accept a copy of the interface with the same method set, while a function
// type requires the two types to be IDENTICAL. So if the tool ever declares
// its own Source instead of aliasing the contract's, the seam breaks here —
// at the tool, not at the far end of the wiring.
var (
	_ func(overseerapi.Source)       = func(Source) {}
	_ func(overseerapi.BulkResult)   = func(BulkResult) {}
	_ func(overseerapi.SearchFilter) = func(SearchFilter) {}
)
