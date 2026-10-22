// SPDX-License-Identifier: MIT

package executionprofile

import (
	"reflect"
	"testing"
)

// TestRemoteBackendIDsIsTheOneList: the remote backends are listed once, and
// the list is the five profiles whose work runs on another host.
func TestRemoteBackendIDsIsTheOneList(t *testing.T) {
	want := []string{"ssh", "k8s", "modal", "daytona", "remote-agezt"}
	if !reflect.DeepEqual(RemoteBackendIDs, want) {
		t.Fatalf("remote backends = %v, want %v", RemoteBackendIDs, want)
	}
	seen := map[string]bool{}
	for _, id := range RemoteBackendIDs {
		if seen[id] {
			t.Fatalf("duplicate remote backend %q", id)
		}
		seen[id] = true
		if !IsRemoteBackend(id) {
			t.Fatalf("%q is listed but not recognized", id)
		}
	}
}

// TestIsRemoteBackend: the predicate names the remote backends and nothing
// else — the warden family and the local profile are not remote.
func TestIsRemoteBackend(t *testing.T) {
	for _, id := range RemoteBackendIDs {
		if !IsRemoteBackend(id) {
			t.Fatalf("%q is a remote backend", id)
		}
	}
	for _, id := range []string{"", " ", "local", "warden", "container", "docker", "unknown", "sshx", "remote", "agezt"} {
		if IsRemoteBackend(id) {
			t.Fatalf("%q is not a remote backend", id)
		}
	}
	// The comparison ignores the case and the surrounding spaces, because the
	// callers that pass a user-supplied profile id promise neither.
	for _, id := range []string{"SSH", "K8s", "Modal", "Daytona", "Remote-Agezt", " ssh ", "\tDAYTONA\n"} {
		if !IsRemoteBackend(id) {
			t.Fatalf("%q is a remote backend", id)
		}
	}
}

// TestInventoryRemoteProfilesUseTheSameIDs: the inventory's remote profiles
// carry the ids the list names, so a profile cannot answer to a name the rest
// of the tree does not know.
func TestInventoryRemoteProfilesUseTheSameIDs(t *testing.T) {
	inv := Build(Options{Tools: []string{"shell", "code_exec", "remote_run"}})
	for _, id := range RemoteBackendIDs {
		p, ok := inv.Find(id)
		if !ok {
			t.Fatalf("the inventory has no %q profile", id)
		}
		if p.ID != id {
			t.Fatalf("the %q profile answers to %q", id, p.ID)
		}
	}
	// The routable ids name nothing outside the known profiles: the local and
	// the warden ones, the container profile and the remote backends — and
	// they name each of them once.
	known := map[string]bool{IDLocal: true, IDWarden: true, "docker": true}
	for _, id := range RemoteBackendIDs {
		known[id] = true
	}
	seen := map[string]bool{}
	for _, id := range RoutableRunProfileIDsFor(inv) {
		if !known[id] {
			t.Fatalf("the routable ids name an unknown profile %q", id)
		}
		if seen[id] {
			t.Fatalf("the routable ids name %q twice", id)
		}
		seen[id] = true
	}
}

// TestRoutableRunProfileIDsForNamesEachBackend: with every remote backend
// active, the routable ids name each of them — under its own name, so a
// lookup that answers with another backend is visible.
func TestRoutableRunProfileIDsForNamesEachBackend(t *testing.T) {
	inv := Build(Options{
		Tools:   []string{"shell", "code_exec", "remote_run"},
		SSH:     SSHConfig{Enabled: true, Target: "deploy@example.com"},
		K8s:     K8sConfig{Enabled: true, Pod: "pod-1"},
		Modal:   ModalConfig{Enabled: true},
		Daytona: DaytonaConfig{Enabled: true, Sandbox: "sandbox-1"},
	})
	routable := RoutableRunProfileIDsFor(inv)
	for _, id := range RemoteBackendIDs {
		found := false
		for _, routableID := range routable {
			if routableID == id {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%q is active but not routable: %v", id, routable)
		}
	}
	// And each of them exactly once.
	counts := map[string]int{}
	for _, id := range routable {
		counts[id]++
	}
	for id, n := range counts {
		if n != 1 {
			t.Fatalf("the routable ids name %q %d times: %v", id, n, routable)
		}
	}
}

// TestRoutableRunProfileIDsForNamesOnlyTheActiveBackend: with one remote
// backend active, the routable ids name that one and no other — so a lookup
// that answers with another backend is visible.
func TestRoutableRunProfileIDsForNamesOnlyTheActiveBackend(t *testing.T) {
	for _, tc := range []struct {
		id   string
		opts Options
	}{
		{IDSSH, Options{SSH: SSHConfig{Enabled: true, Target: "deploy@example.com"}}},
		{IDK8s, Options{K8s: K8sConfig{Enabled: true, Pod: "pod-1"}}},
		{IDModal, Options{Modal: ModalConfig{Enabled: true}}},
		{IDDaytona, Options{Daytona: DaytonaConfig{Enabled: true, Sandbox: "sandbox-1"}}},
	} {
		opts := tc.opts
		opts.Tools = []string{"shell", "code_exec"}
		routable := RoutableRunProfileIDsFor(Build(opts))
		if !containsID(routable, tc.id) {
			t.Fatalf("%q is active but not routable: %v", tc.id, routable)
		}
		for _, other := range RemoteBackendIDs {
			if other == tc.id {
				continue
			}
			if containsID(routable, other) {
				t.Fatalf("%q is not active but is routable: %v", other, routable)
			}
		}
	}
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
