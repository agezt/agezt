// SPDX-License-Identifier: MIT

package creds_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/creds/sigv4"
)

const assumeRoleHappyResponse = `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleResult>
    <Credentials>
      <AccessKeyId>ASIAEXAMPLE</AccessKeyId>
      <SecretAccessKey>wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY</SecretAccessKey>
      <SessionToken>FwoGZXIvYXdzEDS3-temp-token</SessionToken>
      <Expiration>2026-05-29T13:00:00Z</Expiration>
    </Credentials>
  </AssumeRoleResult>
  <ResponseMetadata>
    <RequestId>aaaa-bbbb</RequestId>
  </ResponseMetadata>
</AssumeRoleResponse>`

func TestAssumeRole_HappyPath(t *testing.T) {
	var seenAuth, seenBody, seenContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		seenBody = string(b)
		_, _ = w.Write([]byte(assumeRoleHappyResponse))
	}))
	defer srv.Close()

	got, err := creds.AssumeRole(context.Background(), creds.AssumeRoleParams{
		Region:          "us-east-1",
		BaseCreds:       sigv4.Creds{AccessKeyID: "AKID", SecretAccessKey: "SK"},
		RoleArn:         "arn:aws:iam::123456789012:role/MyRole",
		RoleSessionName: "test-session",
		DurationSeconds: 1800,
		Endpoint:        srv.URL,
		Now:             func() time.Time { return time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("AssumeRole: %v", err)
	}
	if got.Creds.AccessKeyID != "ASIAEXAMPLE" {
		t.Errorf("AccessKeyID=%q", got.Creds.AccessKeyID)
	}
	if got.Creds.SessionToken == "" {
		t.Error("SessionToken empty")
	}
	if !got.Expiration.Equal(time.Date(2026, 5, 29, 13, 0, 0, 0, time.UTC)) {
		t.Errorf("Expiration=%v", got.Expiration)
	}
	// SigV4-signed against service=sts (the whole point of M1.SigV4):
	if !strings.Contains(seenAuth, "/us-east-1/sts/aws4_request") {
		t.Errorf("Authorization missing sts service scope: %q", seenAuth)
	}
	if seenContentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type=%q", seenContentType)
	}
	// Required form params present:
	for _, want := range []string{
		"Action=AssumeRole",
		"Version=2011-06-15",
		"RoleArn=arn%3Aaws%3Aiam%3A%3A123456789012%3Arole%2FMyRole",
		"RoleSessionName=test-session",
		"DurationSeconds=1800",
	} {
		if !strings.Contains(seenBody, want) {
			t.Errorf("body missing %q: %q", want, seenBody)
		}
	}
}

func TestAssumeRole_ExternalIDForwarded(t *testing.T) {
	var seenBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seenBody = string(b)
		_, _ = w.Write([]byte(assumeRoleHappyResponse))
	}))
	defer srv.Close()
	_, err := creds.AssumeRole(context.Background(), creds.AssumeRoleParams{
		Region:     "us-east-1",
		BaseCreds:  sigv4.Creds{AccessKeyID: "AKID", SecretAccessKey: "SK"},
		RoleArn:    "arn:aws:iam::1:role/r",
		ExternalID: "shared-secret-123",
		Endpoint:   srv.URL,
	})
	if err != nil {
		t.Fatalf("AssumeRole: %v", err)
	}
	if !strings.Contains(seenBody, "ExternalId=shared-secret-123") {
		t.Errorf("body missing ExternalId: %q", seenBody)
	}
}

func TestAssumeRole_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`<ErrorResponse><Error><Code>AccessDenied</Code><Message>not allowed</Message></Error></ErrorResponse>`))
	}))
	defer srv.Close()
	_, err := creds.AssumeRole(context.Background(), creds.AssumeRoleParams{
		Region:    "us-east-1",
		BaseCreds: sigv4.Creds{AccessKeyID: "AKID", SecretAccessKey: "SK"},
		RoleArn:   "arn:aws:iam::1:role/x",
		Endpoint:  srv.URL,
	})
	if err == nil {
		t.Fatal("expected error on 403")
	}
	if !strings.Contains(err.Error(), "AccessDenied") {
		t.Errorf("error should include AccessDenied: %v", err)
	}
}

func TestAssumeRole_RejectsMissingFields(t *testing.T) {
	_, err := creds.AssumeRole(context.Background(), creds.AssumeRoleParams{
		BaseCreds: sigv4.Creds{AccessKeyID: "AKID", SecretAccessKey: "SK"},
	})
	if err == nil {
		t.Error("expected error when RoleArn is empty")
	}
	_, err = creds.AssumeRole(context.Background(), creds.AssumeRoleParams{
		RoleArn: "arn:...:role/x",
	})
	if err == nil {
		t.Error("expected error when BaseCreds are empty")
	}
}

// TestAWSAssumeRoleLookup_CachesUntilExpiry is the load-bearing
// behaviour for daemon use: the lookup must not hammer STS on every
// chain probe. It must call STS once, cache, and only re-call once
// the cached creds enter the refreshLeadTime window.
func TestAWSAssumeRoleLookup_CachesUntilExpiry(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		// Each call returns a different SessionToken so we can tell
		// cache hits apart from re-fetches without comparing timestamps.
		body := strings.Replace(
			assumeRoleHappyResponse,
			"FwoGZXIvYXdzEDS3-temp-token",
			fmt.Sprintf("token-call-%d", n),
			1,
		)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	// Pin time so cache decisions are deterministic. First call at
	// 12:00 — cached creds expire at 13:00 (per the response). With
	// refreshLeadTime = 60s, the cached creds stop being valid at
	// 12:59 and beyond.
	nowAt := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	params := creds.AssumeRoleParams{
		Region:    "us-east-1",
		BaseCreds: sigv4.Creds{AccessKeyID: "AKID", SecretAccessKey: "SK"},
		RoleArn:   "arn:aws:iam::1:role/x",
		Endpoint:  srv.URL,
		Now:       func() time.Time { return nowAt },
	}
	lookup := creds.AWSAssumeRoleLookup(params)

	// First credential probe — populates cache.
	if got := lookup("AWS_ACCESS_KEY_ID"); got != "ASIAEXAMPLE" {
		t.Errorf("first AKID = %q", got)
	}
	// Many follow-up probes — all should hit cache.
	for range 5 {
		_ = lookup("AWS_SECRET_ACCESS_KEY")
		_ = lookup("AWS_SESSION_TOKEN")
		_ = lookup("AWS_ACCESS_KEY_ID")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("STS called %d times, want 1 (cache miss-fest)", n)
	}

	// Region passthrough — independent of cache.
	if got := lookup("AWS_REGION"); got != "us-east-1" {
		t.Errorf("region=%q", got)
	}

	// Unrelated name — must return empty so the chain falls through.
	if got := lookup("SOMETHING_ELSE"); got != "" {
		t.Errorf("non-AWS name should return empty, got %q", got)
	}
}
