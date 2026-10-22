// SPDX-License-Identifier: MIT

package executionprofile

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestProfileRowCarriesEveryField: the profile row is published as it is —
// every field is present, the empty optional fields included, and the secret
// policy comes last, the way the operator reads it.
func TestProfileRowCarriesEveryField(t *testing.T) {
	raw, err := json.Marshal(Profile{ID: "bare"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"bare","name":"","summary":"","status":"","routed":false,"requested_isolation":"","effective_isolation":"","degraded":false,"degrade_reason":"","tools":null,"backends":null,"filesystem":"","network":"","environment":"","secrets":"","limits":null,"browser_access":"","cleanup":"","policy_capability":"","notes":null}`
	if string(raw) != want {
		t.Fatalf("the profile row lost a field or moved one:\n%s\n%s", string(raw), want)
	}
}

// TestHealthCheckRowCarriesEveryField: the health check row is published as
// it is — every field is present, the empty optional fields included.
func TestHealthCheckRowCarriesEveryField(t *testing.T) {
	raw, err := json.Marshal(HealthCheck{ID: "ssh.backend"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"ssh.backend","profile_id":"","status":"","title":"","detail":"","next":"","routed":false,"degraded":false,"backend_available":false,"backend":""}`
	if string(raw) != want {
		t.Fatalf("the health check row lost a field or moved one:\n%s\n%s", string(raw), want)
	}
}

// TestSecretPolicyRowCarriesEveryField: the secret policy row keeps every
// field as well.
func TestSecretPolicyRowCarriesEveryField(t *testing.T) {
	raw, err := json.Marshal(SecretPolicy{Mode: "deny"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"mode":"deny","scope":"","values_forwarded":false,"metadata_forwarded":false,"valid":false,"detail":""}`
	if string(raw) != want {
		t.Fatalf("the secret policy row lost a field or moved one:\n%s\n%s", string(raw), want)
	}
	if !strings.Contains(string(raw), `"detail":""`) {
		t.Fatal("the secret policy row keeps the empty detail")
	}
}
