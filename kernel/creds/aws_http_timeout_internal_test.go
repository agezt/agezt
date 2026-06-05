// SPDX-License-Identifier: MIT

package creds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/creds/sigv4"
)

// TestAssumeRole_HTTPTimeoutBounded pins M465: an AWS credential fetch against a
// stalled endpoint must time out, not hang. These paths previously used
// http.DefaultClient (no timeout) with a background context (no deadline), so a
// black-holed STS/SSO/web-identity endpoint could hang daemon startup forever.
// AssumeRole (STS) is the behavioral witness; sso.go and web_identity.go share the
// identical fix (the bounded client built from credentialHTTPTimeout).
func TestAssumeRole_HTTPTimeoutBounded(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-block // never respond within the timeout
	}))
	defer srv.Close()
	defer close(block) // unblock the handler so srv.Close() can return

	orig := credentialHTTPTimeout
	credentialHTTPTimeout = 100 * time.Millisecond
	defer func() { credentialHTTPTimeout = orig }()

	done := make(chan error, 1)
	go func() {
		_, err := AssumeRole(context.Background(), AssumeRoleParams{
			Region:          "us-east-1",
			BaseCreds:       sigv4.Creds{AccessKeyID: "AKID", SecretAccessKey: "SK"},
			RoleArn:         "arn:aws:iam::1:role/r",
			RoleSessionName: "s",
			Endpoint:        srv.URL,
			Now:             func() time.Time { return time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC) },
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a timeout error from a hanging endpoint, got nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("AssumeRole did not time out against a hanging endpoint: the credential-fetch HTTP client has no timeout")
	}
}
