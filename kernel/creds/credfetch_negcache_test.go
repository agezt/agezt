// SPDX-License-Identifier: MIT

package creds_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/creds/sigv4"
)

// failingCredServer returns an httptest server that refuses every credential
// fetch with 403 and counts the attempts. One logical chain resolution probes
// AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and AWS_SESSION_TOKEN in sequence,
// so a DOOMED fetch (revoked base creds, refused or black-holed endpoint)
// must be attempted ONCE for that resolution — not once per name. The
// package's documented posture is "No retry / backoff", and the IMDS layer
// already enforces exactly that with its negCache window.
func failingCredServer(t *testing.T, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// assertNamesFallThrough probes the three credential names and asserts each
// returns empty (the layer must stay transparent to ChainLookup on failure).
func assertNamesFallThrough(t *testing.T, lookup func(string) string) {
	t.Helper()
	for _, name := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"} {
		if got := lookup(name); got != "" {
			t.Fatalf("%s = %q on a failing endpoint, want empty (chain falls through)", name, got)
		}
	}
}

// TestAWSAssumeRoleLookup_FailedFetchAttemptedOncePerResolution: one failed
// chain resolution must cost ONE AssumeRole attempt. Regression guard: the
// doomed fetch used to be re-attempted for every credential name — three
// doomed network calls (each up to credentialHTTPTimeout) per resolution,
// repeated for the daemon's lifetime.
func TestAWSAssumeRoleLookup_FailedFetchAttemptedOncePerResolution(t *testing.T) {
	var calls atomic.Int32
	srv := failingCredServer(t, &calls)
	lookup := creds.AWSAssumeRoleLookup(creds.AssumeRoleParams{
		Region:    "us-east-1",
		BaseCreds: sigv4.Creds{AccessKeyID: "AKID", SecretAccessKey: "SK"},
		RoleArn:   "arn:aws:iam::123456789012:role/x",
		Endpoint:  srv.URL,
	})
	assertNamesFallThrough(t, lookup)
	if n := calls.Load(); n != 1 {
		t.Errorf("one failed chain resolution made %d AssumeRole attempts, want 1: the doomed fetch "+
			"is re-attempted for every credential name, amplifying a slow failure on every lookup", n)
	}
}

// TestAWSWebIdentityLookup_FailedFetchAttemptedOncePerResolution: same
// contract for the web-identity (IRSA) layer, which shares the cache shape.
func TestAWSWebIdentityLookup_FailedFetchAttemptedOncePerResolution(t *testing.T) {
	var calls atomic.Int32
	srv := failingCredServer(t, &calls)
	lookup := creds.AWSWebIdentityLookup(creds.WebIdentityParams{
		Region:    "us-west-2",
		RoleArn:   "arn:aws:iam::123456789012:role/x",
		TokenFile: writeTokenFile(t, "jwt"),
		Endpoint:  srv.URL,
	})
	assertNamesFallThrough(t, lookup)
	if n := calls.Load(); n != 1 {
		t.Errorf("one failed chain resolution made %d web-identity attempts, want 1", n)
	}
}

// TestAWSSSOLookup_FailedFetchAttemptedOncePerResolution: same contract for
// the SSO portal layer (valid cached bearer token, failing portal).
func TestAWSSSOLookup_FailedFetchAttemptedOncePerResolution(t *testing.T) {
	var calls atomic.Int32
	srv := failingCredServer(t, &calls)
	cacheDir := t.TempDir()
	startURL := "https://org.awsapps.com/start"
	writeSSOToken(t, cacheDir, startURL, "test-access-token", time.Now().Add(time.Hour))
	lookup := creds.AWSSSOLookup(creds.SSOParams{
		StartURL:  startURL,
		Region:    "us-east-1",
		AccountID: "123456789012",
		RoleName:  "AdminRole",
		CacheDir:  cacheDir,
		Endpoint:  srv.URL,
	})
	assertNamesFallThrough(t, lookup)
	if n := calls.Load(); n != 1 {
		t.Errorf("one failed chain resolution made %d SSO portal attempts, want 1", n)
	}
}

// TestAWSAssumeRoleLookup_SuppressionWindowExpires: the negative cache is a
// suppression WINDOW, not a permanent blackout — inside it the layer must not
// re-attempt the doomed fetch at all; well past it the layer tries again
// (once), and the window re-arms around that new failure. Drives the clock
// through the Now seam so no real waiting is involved.
func TestAWSAssumeRoleLookup_SuppressionWindowExpires(t *testing.T) {
	var calls atomic.Int32
	srv := failingCredServer(t, &calls)
	nowAt := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	lookup := creds.AWSAssumeRoleLookup(creds.AssumeRoleParams{
		Region:    "us-east-1",
		BaseCreds: sigv4.Creds{AccessKeyID: "AKID", SecretAccessKey: "SK"},
		RoleArn:   "arn:aws:iam::123456789012:role/x",
		Endpoint:  srv.URL,
		Now:       func() time.Time { return nowAt },
	})

	_ = lookup("AWS_ACCESS_KEY_ID") // attempt 1: the doomed fetch
	// Still inside the suppression window: the same doomed fetch must not re-run.
	nowAt = nowAt.Add(time.Second)
	_ = lookup("AWS_ACCESS_KEY_ID")
	if n := calls.Load(); n != 1 {
		t.Fatalf("re-lookup inside the suppression window made %d attempts, want 1", n)
	}
	// Well past the window: the layer tries again — once.
	nowAt = nowAt.Add(time.Hour)
	_ = lookup("AWS_ACCESS_KEY_ID")
	if n := calls.Load(); n != 2 {
		t.Fatalf("lookup after the suppression window made %d attempts, want 2", n)
	}
	// The failure re-arms the window.
	nowAt = nowAt.Add(time.Second)
	_ = lookup("AWS_ACCESS_KEY_ID")
	if n := calls.Load(); n != 2 {
		t.Fatalf("re-lookup inside the re-armed suppression window made %d attempts, want 2", n)
	}
}
