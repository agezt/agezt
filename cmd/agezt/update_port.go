// SPDX-License-Identifier: MIT

package main

import (
	"github.com/agezt/agezt/kernel/contract/updateapi"
	"github.com/agezt/agezt/kernel/update"
)

// updatePort converts the concrete update engine to the CONTRACT's port
// (W5.4c).
//
// The conversion happens HERE, where the value is still concrete, because a
// nil *update.Service handed to an interface parameter is a **NON-nil**
// interface: the holder would hold an engine that is not there, the
// "update is disabled" answer would never fire, and the engine's methods
// would be called on a nil receiver.
//
// This is the same rule the board's port follows (W3.59).
func updatePort(svc *update.Service) updateapi.Backend {
	if svc == nil {
		return nil
	}
	return svc
}
