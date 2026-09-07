// SPDX-License-Identifier: MIT

package creds_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/creds"
)

// TestGetSSORoleCredentials_MissingSessionTokenRejected: GetRoleCredentials
// returns temporary STS credentials — they ALWAYS carry a session token. The
// sibling parsers (sts.go parseAssumeRoleResponse, web_identity.go
// parseWebIdentityResponse) reject responses missing it, because a token-less
// assumed-role credential cannot sign: accepted, it splits the credential
// across chain sources (AKID/SECRET from SSO, session token from env/IMDS)
// and produces cryptic SignatureDoesNotMatch failures downstream.
func TestGetSSORoleCredentials_MissingSessionTokenRejected(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		expMs := time.Date(2026, 5, 29, 13, 30, 0, 0, time.UTC).UnixMilli()
		// A mangled/proxied response: valid AKID/SECRET/expiration but NO
		// sessionToken field.
		fmt.Fprintf(w, `{"roleCredentials":{"accessKeyId":"ASIA-SSO","secretAccessKey":"sk-sso","expiration":%d}}`, expMs)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	startURL := "https://org.awsapps.com/start"
	writeSSOToken(t, cacheDir, startURL, "test-access-token", time.Now().Add(time.Hour))

	got, err := creds.GetSSORoleCredentials(context.Background(), creds.SSOParams{
		StartURL:  startURL,
		Region:    "us-east-1",
		AccountID: "123456789012",
		RoleName:  "AdminRole",
		CacheDir:  cacheDir,
		Endpoint:  srv.URL,
	})
	if err == nil {
		t.Fatalf("response without sessionToken was accepted (attempts=%d): got %+v — "+
			"a token-less assumed-role credential cannot sign; it must be rejected like the "+
			"STS and web-identity parsers do", calls.Load(), got)
	}
	if got != nil {
		t.Errorf("on rejection the result must be nil, got %+v", got)
	}
}
