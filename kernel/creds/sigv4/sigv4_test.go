// SPDX-License-Identifier: MIT

package sigv4_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/creds/sigv4"
)

// TestSignRequest_ServiceScopedCorrectly is the load-bearing test
// for M1.SigV4: the extracted signer must produce *different*
// signatures for the same request when the service code changes,
// because the service is mixed into the signing-key derivation
// chain. If a refactor accidentally hard-codes a service again
// (or drops the parameter), this test fails.
func TestSignRequest_ServiceScopedCorrectly(t *testing.T) {
	body := []byte(`{}`)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	creds := sigv4.Creds{AccessKeyID: "AKID", SecretAccessKey: "SK"}

	sign := func(service, host string) string {
		req, _ := http.NewRequest("POST", "https://"+host+"/", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if err := sigv4.SignRequest(req, service, "us-east-1", body, creds, now); err != nil {
			t.Fatalf("SignRequest(%s): %v", service, err)
		}
		return req.Header.Get("Authorization")
	}

	bedrockSig := sign("bedrock", "bedrock-runtime.us-east-1.amazonaws.com")
	stsSig := sign("sts", "sts.us-east-1.amazonaws.com")
	ssoSig := sign("awsssoportal", "portal.sso.us-east-1.amazonaws.com")

	if bedrockSig == stsSig {
		t.Error("bedrock and sts produced identical Authorization headers — service not mixed into signing key")
	}
	if stsSig == ssoSig {
		t.Error("sts and sso produced identical Authorization headers — service not mixed into signing key")
	}

	// Each must carry its service code in the credential scope so
	// AWS can recompute the same signing key server-side.
	if !strings.Contains(bedrockSig, "/us-east-1/bedrock/aws4_request") {
		t.Errorf("bedrock scope missing: %q", bedrockSig)
	}
	if !strings.Contains(stsSig, "/us-east-1/sts/aws4_request") {
		t.Errorf("sts scope missing: %q", stsSig)
	}
	if !strings.Contains(ssoSig, "/us-east-1/awsssoportal/aws4_request") {
		t.Errorf("sso scope missing: %q", ssoSig)
	}
}

// TestSignRequest_Deterministic locks in that signing is pure —
// re-signing the same request with the same inputs produces the
// same bytes. Catches accidental introduction of nondeterminism
// (e.g. someone adding time.Now() inside the algorithm).
func TestSignRequest_Deterministic(t *testing.T) {
	body := []byte(`{"x":1}`)
	now := time.Date(2026, 5, 29, 12, 34, 56, 0, time.UTC)
	creds := sigv4.Creds{AccessKeyID: "AKID", SecretAccessKey: "SK"}

	one, _ := http.NewRequest("POST", "https://sts.us-east-1.amazonaws.com/", bytes.NewReader(body))
	one.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := sigv4.SignRequest(one, "sts", "us-east-1", body, creds, now); err != nil {
		t.Fatalf("first sign: %v", err)
	}
	two, _ := http.NewRequest("POST", "https://sts.us-east-1.amazonaws.com/", bytes.NewReader(body))
	two.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := sigv4.SignRequest(two, "sts", "us-east-1", body, creds, now); err != nil {
		t.Fatalf("second sign: %v", err)
	}
	if a, b := one.Header.Get("Authorization"), two.Header.Get("Authorization"); a != b {
		t.Errorf("non-deterministic:\n %s\n %s", a, b)
	}
}

// TestSignRequest_SessionTokenSigned ensures STS-issued temporary
// creds attach AND sign the X-Amz-Security-Token header. Critical
// for any caller using AssumeRole / SSO / IMDS / credential_process
// since those always produce session tokens.
func TestSignRequest_SessionTokenSigned(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://sts.us-east-1.amazonaws.com/", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	creds := sigv4.Creds{
		AccessKeyID:     "AKID",
		SecretAccessKey: "SK",
		SessionToken:    "FwoG-temp-token",
	}
	if err := sigv4.SignRequest(req, "sts", "us-east-1", nil, creds, time.Now()); err != nil {
		t.Fatalf("SignRequest: %v", err)
	}
	if req.Header.Get("X-Amz-Security-Token") != "FwoG-temp-token" {
		t.Errorf("token not attached")
	}
	if !strings.Contains(req.Header.Get("Authorization"), "x-amz-security-token") {
		t.Errorf("token not in SignedHeaders: %q", req.Header.Get("Authorization"))
	}
}

func TestSignRequest_RequiresService(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://x/", nil)
	creds := sigv4.Creds{AccessKeyID: "AKID", SecretAccessKey: "SK"}
	if err := sigv4.SignRequest(req, "", "us-east-1", nil, creds, time.Now()); err == nil {
		t.Error("expected error when service is empty")
	}
}

func TestCanonicalQuery_Sorts(t *testing.T) {
	got := sigv4.CanonicalQuery(map[string][]string{
		"Version": {"2011-06-15"},
		"Action":  {"AssumeRole"},
	})
	want := "Action=AssumeRole&Version=2011-06-15"
	if got != want {
		t.Errorf("CanonicalQuery = %q, want %q", got, want)
	}
}

func TestAWSURIEncode_Tilde(t *testing.T) {
	// AWS keeps `~` unreserved; net/url's QueryEscape encodes it.
	// Regression-guard the documented divergence.
	if sigv4.AWSURIEncode("~", true) != "~" {
		t.Error("AWSURIEncode must leave ~ unencoded")
	}
}
