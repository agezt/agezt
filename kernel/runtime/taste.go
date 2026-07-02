// SPDX-License-Identifier: MIT

package runtime

import (
	"github.com/agezt/agezt/kernel/seat"
	"github.com/agezt/agezt/kernel/taste"
)

// Taste returns the curated "what good looks like" exemplar store.
func (k *Kernel) Taste() *taste.Store { return k.taste }

// Seats returns the execution-seat catalog (seeded built-ins + custom seats).
func (k *Kernel) Seats() *seat.Store { return k.seat }
