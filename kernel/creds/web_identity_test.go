// SPDX-License-Identifier: MIT

package creds_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/creds"
)

const webIdentityHappyResponse = `<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleWithWebIdentityResult>
    <Credentials>
      <AccessKeyId>ASIAWEBIDENT</AccessKeyId>
      <SecretAccessKey>web-secret-key</SecretAccessKey>
      <SessionToken>web-session-token</SessionToken>
      <Expiration>2026-05-29T13:00:00Z</Expiration>
    </Credentials>
    <SubjectFromWebIdentityToken>system:serviceaccount:default:agezt</SubjectFromWebIdentityToken>
  </AssumeRoleWithWebIdentityResult>
  <ResponseMetadata>
    <RequestId>cccc-dddd</RequestId>
  </ResponseMetadata>
</AssumeRoleWithWebIdentityResponse>`

// writeTokenFile drops an OIDC token into a temp file and returns its path.
func writeTokenFile(t *testing.T, token string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	return path
}

func TestAssumeRoleWithWebIdentity_HappyPathIsUnsigned(t *testing.T) {
	var seenAuth, seenBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		seenBody = string(b)
		_, _ = w.Write([]byte(webIdentityHappyResponse))
	}))
	defer srv.Close()

	tokenPath := writeTokenFile(t, "eyJ-fake-oidc-jwt")
	got, err := creds.AssumeRoleWithWebIdentity(context.Background(), creds.WebIdentityParams{
		Region:          "us-west-2",
		RoleArn:         "arn:aws:iam::123456789012:role/EksPodRole",
		RoleSessionName: "agezt-pod",
		TokenFile:       tokenPath,
		Endpoint:        srv.URL,
	})
	if err != nil {
		t.Fatalf("AssumeRoleWithWebIdentity: %v", err)
	}
	if got.Creds.AccessKeyID != "ASIAWEBIDENT" {
		t.Errorf("AccessKeyID=%q", got.Creds.AccessKeyID)
	}
	if got.Creds.SessionToken != "web-session-token" {
		t.Errorf("SessionToken=%q", got.Creds.SessionToken)
	}
	if !got.Expiration.Equal(time.Date(2026, 5, 29, 13, 0, 0, 0, time.UTC)) {
		t.Errorf("Expiration=%v", got.Expiration)
	}
	// THE keyless property: no SigV4 signing — the OIDC token is the auth.
	if seenAuth != "" {
		t.Errorf("request was signed (Authorization=%q); web identity must be UNSIGNED", seenAuth)
	}
	// The token-file contents must be forwarded as WebIdentityToken, with
	// the right Action.
	for _, want := range []string{
		"Action=AssumeRoleWithWebIdentity",
		"Version=2011-06-15",
		"WebIdentityToken=eyJ-fake-oidc-jwt",
		"RoleArn=arn%3Aaws%3Aiam%3A%3A123456789012%3Arole%2FEksPodRole",
		"RoleSessionName=agezt-pod",
	} {
		if !strings.Contains(seenBody, want) {
			t.Errorf("body missing %q: %q", want, seenBody)
		}
	}
}

func TestAssumeRoleWithWebIdentity_MissingTokenFileErrors(t *testing.T) {
	_, err := creds.AssumeRoleWithWebIdentity(context.Background(), creds.WebIdentityParams{
		RoleArn:   "arn:aws:iam::1:role/r",
		TokenFile: filepath.Join(t.TempDir(), "does-not-exist"),
	})
	if err == nil || !strings.Contains(err.Error(), "token file") {
		t.Fatalf("err=%v, want a token-file read error", err)
	}
}

func TestAssumeRoleWithWebIdentity_EmptyTokenFileErrors(t *testing.T) {
	_, err := creds.AssumeRoleWithWebIdentity(context.Background(), creds.WebIdentityParams{
		RoleArn:   "arn:aws:iam::1:role/r",
		TokenFile: writeTokenFile(t, "   \n"),
	})
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("err=%v, want an empty-token-file error", err)
	}
}

func TestAssumeRoleWithWebIdentity_APIErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<ErrorResponse><Error><Code>AccessDenied</Code></Error></ErrorResponse>`))
	}))
	defer srv.Close()

	_, err := creds.AssumeRoleWithWebIdentity(context.Background(), creds.WebIdentityParams{
		RoleArn:   "arn:aws:iam::1:role/r",
		TokenFile: writeTokenFile(t, "jwt"),
		Endpoint:  srv.URL,
	})
	if err == nil || !strings.Contains(err.Error(), "AccessDenied") {
		t.Fatalf("err=%v, want the STS AccessDenied surfaced", err)
	}
}

func TestAWSWebIdentityLookup_CachesUntilExpiry(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		body := strings.Replace(webIdentityHappyResponse, "web-session-token",
			fmt.Sprintf("token-call-%d", n), 1)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	nowAt := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	lookup := creds.AWSWebIdentityLookup(creds.WebIdentityParams{
		Region:    "us-west-2",
		RoleArn:   "arn:aws:iam::1:role/x",
		TokenFile: writeTokenFile(t, "jwt"),
		Endpoint:  srv.URL,
		Now:       func() time.Time { return nowAt },
	})

	if got := lookup("AWS_ACCESS_KEY_ID"); got != "ASIAWEBIDENT" {
		t.Errorf("first AKID=%q", got)
	}
	for range 5 {
		_ = lookup("AWS_SECRET_ACCESS_KEY")
		_ = lookup("AWS_SESSION_TOKEN")
		_ = lookup("AWS_ACCESS_KEY_ID")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("STS called %d times, want 1 (cache should hold)", n)
	}
	if got := lookup("AWS_REGION"); got != "us-west-2" {
		t.Errorf("region=%q", got)
	}
	// Unrelated name → empty so the chain falls through.
	if got := lookup("SOMETHING_ELSE"); got != "" {
		t.Errorf("non-AWS name should be empty, got %q", got)
	}
}

// TestAWSWebIdentityLookup_FailureFallsThrough: an STS failure makes every
// credential name return empty so ChainLookup continues to the next source,
// rather than erroring the whole daemon.
func TestAWSWebIdentityLookup_FailureFallsThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	lookup := creds.AWSWebIdentityLookup(creds.WebIdentityParams{
		RoleArn:   "arn:aws:iam::1:role/x",
		TokenFile: writeTokenFile(t, "jwt"),
		Endpoint:  srv.URL,
	})
	if got := lookup("AWS_ACCESS_KEY_ID"); got != "" {
		t.Errorf("on STS failure AKID should be empty (chain falls through), got %q", got)
	}
}
