// SPDX-License-Identifier: MIT

package controlplane

// roster_crud_internal.go keeps managedSubagentDirectCallError, the native
// wrapper around the operator-facing error string for an action sent to a
// managed sub-agent instead of its manager.

import (
	approster "github.com/agezt/agezt/kernel/app/roster"
	"github.com/agezt/agezt/kernel/roster"
)

func managedSubagentDirectCallError(p roster.Profile, action string) string {
	return approster.ManagedDirectCallError(p, action)
}
