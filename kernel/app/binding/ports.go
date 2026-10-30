// SPDX-License-Identifier: MIT

// Package binding hands the daemon's handle to the family that owns the
// ports.
//
// The handle is the daemon's own kernel. A family fills its OWN ports from
// it, and the adapter that holds the handle never names the runtime — Go does
// not need the name to hand a value over. This bridge is that hand-over, and
// it lives in the application layer because the application owns the
// families' ports.
package binding

import "github.com/agezt/agezt/kernel/runtime"

// Fill fills a family's ports from the daemon's handle.
//
// The family that owns the ports fills them; this bridge only hands the
// handle to the right family. A handle that is not the daemon's own yields
// the family's EMPTY ports, and an empty port refuses rather than calling
// into nothing.
func Fill[T any](handle any, fill func(*runtime.Kernel) T) T {
	k, ok := handle.(*runtime.Kernel)
	if !ok {
		var zero T
		return zero
	}
	return fill(k)
}
