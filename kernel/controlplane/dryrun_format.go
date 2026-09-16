// SPDX-License-Identifier: MIT
//
// kernel/controlplane dry-run microcent USD formatter (formatMicrocentsUSD).
// Extracted from dryrun.go during Day 211 god-file refactor (#100).
// Public API unchanged.
package controlplane

import (
	"fmt"
)

func formatMicrocentsUSD(mc int64) string {
	return fmt.Sprintf("$%.2f", float64(mc)/microcentsPerUSD)
}
