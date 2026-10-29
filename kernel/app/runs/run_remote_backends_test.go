// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/executionprofile"
)

// TestRunPathKnowsEveryRemoteBackend: the run path resolves the remote
// backends by the names the execution profile list gives, and each one
// reports its own missing backend — so a name cannot be swapped for another,
// and a new remote backend cannot be added to the list without the run path
// learning it.
func TestRunPathKnowsEveryRemoteBackend(t *testing.T) {
	for _, name := range []string{"AGEZT_EXEC_SSH", "AGEZT_EXEC_K8S", "AGEZT_EXEC_MODAL", "AGEZT_EXEC_DAYTONA"} {
		t.Setenv(name, "")
	}
	k := runKernel(t)
	want := map[string]string{
		executionprofile.IDSSH:         "requires an active SSH backend",
		executionprofile.IDK8s:         "requires an active Kubernetes backend",
		executionprofile.IDModal:       "requires an active Modal backend",
		executionprofile.IDDaytona:     "requires an active Daytona backend",
		executionprofile.IDRemoteAgezt: "requires configured AGEZT peers",
	}
	for _, id := range executionprofile.RemoteBackendIDs {
		_, _, _, err := applyExecutionProfile(context.Background(), RunPortsFromKernel(k), id)
		if err == nil || !strings.Contains(err.Error(), want[id]) {
			t.Fatalf("%q: error = %v, want the %q refusal", id, err, want[id])
		}
	}
	// The profile id is matched without regard to the case, as before.
	if _, _, _, err := applyExecutionProfile(context.Background(), RunPortsFromKernel(k), "SSH"); err == nil || !strings.Contains(err.Error(), want[executionprofile.IDSSH]) {
		t.Fatalf("the upper-case id: error = %v, want the SSH refusal", err)
	}
}
