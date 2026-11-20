// SPDX-License-Identifier: MIT

package controlplane

// The console's route table is checked against the daemon's own registry: the
// console routes operations BY NAME, so a name the daemon does not register is
// a route that answers 502 in production — the browser's panel silently
// breaks, and nothing else would notice.

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

// TestEveryConsoleRouteOperationIsRegistered: every operation the console
// binds is one the daemon answers.
func TestEveryConsoleRouteOperationIsRegistered(t *testing.T) {
	registered := map[string]bool{}
	for name := range commandRegistry {
		registered[name] = true
	}
	for _, operation := range registeredAppOperations() {
		registered[operation.Spec().Name] = true
	}
	if len(registered) == 0 {
		t.Fatal("no registered operations")
	}
	for _, route := range opapi.ConsoleRoutes() {
		if !registered[route.Op] {
			t.Errorf("the console serves %s at %s, but the daemon registers no %q operation",
				route.Op, route.Path, route.Op)
		}
	}
}
