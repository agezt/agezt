// SPDX-License-Identifier: MIT

package runtime

import (
	"github.com/agezt/agezt/kernel/contract/seatapi"
	"github.com/agezt/agezt/kernel/contract/tasteapi"
)

// Taste returns the curated "what good looks like" exemplar store.
//
// It is the CONTRACT's port, not a module's store (W3.52): the library's file
// is the daemon's business and the kernel may not name the module that keeps
// it. A host that was given no library holds a nil port and injects no
// exemplars.
func (k *Kernel) Taste() tasteapi.Store { return k.taste }

// Seats returns the execution-seat catalog (seeded built-ins + custom seats).
func (k *Kernel) Seats() seatapi.Store { return k.seat }
