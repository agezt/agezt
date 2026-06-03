// SPDX-License-Identifier: MIT

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCheckMesh_NoPeers: with no AGEZT_PEERS, the mesh check is an informational OK
// (single-node), never a warning.
func TestCheckMesh_NoPeers(t *testing.T) {
	t.Setenv("AGEZT_PEERS", "")
	c := checkMesh()
	if c.Status != statusOK {
		t.Errorf("no peers should be OK, got %s: %s", c.Status.label(), c.Detail)
	}
	if !strings.Contains(c.Detail, "single-node") {
		t.Errorf("detail = %q", c.Detail)
	}
}

// TestCheckMesh_AllReachable: every configured peer healthy → OK naming the peers.
func TestCheckMesh_AllReachable(t *testing.T) {
	srvA := healthServer(t, "", "1.0", 2)
	defer srvA.Close()
	srvB := healthServer(t, "", "1.0", 3)
	defer srvB.Close()
	t.Setenv("AGEZT_PEERS", "alpha="+srvA.URL+",bravo="+srvB.URL)

	c := checkMesh()
	if c.Status != statusOK {
		t.Fatalf("all reachable should be OK, got %s: %s", c.Status.label(), c.Detail)
	}
	if !strings.Contains(c.Detail, "2 peer(s) reachable") || !strings.Contains(c.Detail, "alpha") {
		t.Errorf("detail = %q", c.Detail)
	}
}

// TestCheckMesh_SomeUnreachable: a down peer is a WARN (local node fine, mesh
// degraded) that names the down peer and carries a remediation hint.
func TestCheckMesh_SomeUnreachable(t *testing.T) {
	srvA := healthServer(t, "", "1.0", 1)
	defer srvA.Close()
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer dead.Close()
	t.Setenv("AGEZT_PEERS", "alpha="+srvA.URL+",bravo="+dead.URL)

	c := checkMesh()
	if c.Status != statusWarn {
		t.Fatalf("a down peer should WARN, got %s: %s", c.Status.label(), c.Detail)
	}
	if !strings.Contains(c.Detail, "1/2 peer(s) unreachable") || !strings.Contains(c.Detail, "bravo") {
		t.Errorf("detail should name the down peer: %q", c.Detail)
	}
	if c.Hint == "" {
		t.Error("a degraded mesh WARN should carry a remediation hint")
	}
}

// TestCheckMesh_MalformedSpec: a bad AGEZT_PEERS is a WARN, not a crash.
func TestCheckMesh_MalformedSpec(t *testing.T) {
	t.Setenv("AGEZT_PEERS", "broken-no-equals")
	c := checkMesh()
	if c.Status != statusWarn {
		t.Errorf("malformed spec should WARN, got %s", c.Status.label())
	}
	if !strings.Contains(c.Detail, "malformed") {
		t.Errorf("detail = %q", c.Detail)
	}
}

// TestCheckMeshHopLimit_ValidOverride: a valid AGEZT_MESH_MAX_HOPS is reported OK with
// its effective value (M213).
func TestCheckMeshHopLimit_ValidOverride(t *testing.T) {
	t.Setenv("AGEZT_MESH_MAX_HOPS", "4")
	c := checkMeshHopLimit()
	if c.Status != statusOK {
		t.Fatalf("valid override should be OK, got %s: %s", c.Status.label(), c.Detail)
	}
	if !strings.Contains(c.Detail, "= 4") {
		t.Errorf("detail should report the effective limit: %q", c.Detail)
	}
}

// TestCheckMeshHopLimit_InvalidWarns: an invalid AGEZT_MESH_MAX_HOPS (which the daemon
// silently ignores) is surfaced as a WARN with a fix hint.
func TestCheckMeshHopLimit_InvalidWarns(t *testing.T) {
	for _, bad := range []string{"abc", "0", "-3", "9999"} {
		t.Setenv("AGEZT_MESH_MAX_HOPS", bad)
		c := checkMeshHopLimit()
		if c.Status != statusWarn {
			t.Errorf("%q should WARN, got %s", bad, c.Status.label())
		}
		if !strings.Contains(c.Detail, "invalid") || c.Hint == "" {
			t.Errorf("%q: detail=%q hint=%q", bad, c.Detail, c.Hint)
		}
	}
}
