// SPDX-License-Identifier: MIT

package controlplane

// roster_crud_internal.go keeps the native wrappers around the application
// profile-write helpers: hierarchy validation for the lifecycle and agent-tool
// paths, and managedSubagentDirectCallError (the operator-facing error string
// when someone tries to send an action to a managed sub-agent instead of its
// manager).

import (
	approster "github.com/agezt/agezt/kernel/app/roster"
	"github.com/agezt/agezt/kernel/roster"
)

// validateAgentHierarchyRefs checks owner/parent references against the live
// roster for the native lifecycle and agent-tool callers.
func (s *Server) validateAgentHierarchyRefs(p roster.Profile) error {
	return approster.ValidateHierarchyRefs(p, s.k.Roster().Get)
}

func managedSubagentDirectCallError(p roster.Profile, action string) string {
	return approster.ManagedDirectCallError(p, action)
}
