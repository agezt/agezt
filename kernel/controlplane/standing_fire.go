// SPDX-License-Identifier: MIT
package controlplane

// StandingFirer fires one standing order on demand (M765).
//
// The daemon's resident standing-order runner supplies it: the callback closes
// over the daemon's fire path and its context, so this package stays decoupled
// from the run launcher. It returns false when the id is unknown.
type StandingFirer = func(id string) bool
