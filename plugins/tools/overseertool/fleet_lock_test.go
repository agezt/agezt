// SPDX-License-Identifier: MIT

package overseertool

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/roster"
)

// TestFleetLock_RefusesAgentEditAndCreate verifies the V-012 opt-in guard: when
// the fleet lock is on, the agent-reachable EditAgent/CreateAgent refuse before
// touching the kernel (so a nil-kernel source is a valid way to prove the guard
// fires first). Off by default, the guard is absent and the default-allow
// posture is unchanged.
func TestFleetLock_RefusesAgentEditAndCreate(t *testing.T) {
	locked := &kernelSource{fleetLock: true} // nil kernel: guard must return before any k deref

	if _, err := locked.EditAgent("some-agent", roster.Profile{Name: "x"}); err == nil ||
		!strings.Contains(err.Error(), "locked") {
		t.Fatalf("EditAgent under lock: err = %v, want a 'locked' refusal", err)
	}
	if _, err := locked.CreateAgent(roster.Profile{Slug: "new-agent"}); err == nil ||
		!strings.Contains(err.Error(), "locked") {
		t.Fatalf("CreateAgent under lock: err = %v, want a 'locked' refusal", err)
	}
	if _, err := locked.DeleteAgent("some-agent"); err == nil ||
		!strings.Contains(err.Error(), "locked") {
		t.Fatalf("DeleteAgent under lock: err = %v, want a 'locked' refusal", err)
	}
}

func TestFleetLockEnabled_ParsesEnv(t *testing.T) {
	for _, on := range []string{"1", "on", "ON", "true", "True", "yes"} {
		t.Setenv("AGEZT_OVERSEER_FLEET_LOCK", on)
		if !fleetLockEnabled() {
			t.Errorf("value %q should enable the fleet lock", on)
		}
	}
	for _, off := range []string{"", "0", "off", "false", "no", "garbage"} {
		t.Setenv("AGEZT_OVERSEER_FLEET_LOCK", off)
		if fleetLockEnabled() {
			t.Errorf("value %q should NOT enable the fleet lock (default-allow)", off)
		}
	}
}
