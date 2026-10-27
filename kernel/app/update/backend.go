// SPDX-License-Identifier: MIT

// Package update owns operator update use cases over the verified update engine.
package update

import (
	"github.com/agezt/agezt/kernel/contract/updateapi"
)

// Backend retains the existing update engine's verification and drain contract.
//
// The port is the CONTRACT's (W5.4c): the control plane and the REST surface
// are adapters, and an adapter may not name the update module. This is an
// ALIAS of the contract's port, not a second shape of it — two interfaces
// with the same verbs would drift apart on their own.
type Backend = updateapi.Backend
