// SPDX-License-Identifier: MIT

package creds_test

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
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

// TestGetSSORoleCredentials_MissingExpiresAtRejected: the AWS CLI v2 SSO
// token cache always carries `expiresAt` — it is how the CLI itself decides
// to refresh. A cache file with an accessToken but NO expiresAt is truncated
// or corrupt; treating it as never-expiring sends an ancient bearer token to
// the portal (surfacing as an opaque 401 excerpt) instead of failing closed
// with the re-login guidance. The cache must be rejected at read time,
// before any network call.
func TestGetSSORoleCredentials_MissingExpiresAtRejected(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		expMs := time.Now().Add(time.Hour).UnixMilli()
		fmt.Fprintf(w, `{"roleCredentials":{"accessKeyId":"ASIA-SSO","secretAccessKey":"sk-sso","sessionToken":"st-sso","expiration":%d}}`, expMs)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	startURL := "https://org.awsapps.com/start"
	// Mirror the documented cache convention: sha1(startURL) + ".json".
	sum := sha1.Sum([]byte(startURL))
	name := filepath.Join(cacheDir, hex.EncodeToString(sum[:])+".json")
	// accessToken present, expiresAt MISSING (truncated write / corruption).
	body := `{"startUrl":"` + startURL + `","region":"us-east-1","accessToken":"tok-no-expiry"}`
	if err := os.WriteFile(name, []byte(body), 0o600); err != nil {
		t.Fatalf("write malformed cache: %v", err)
	}

	got, err := creds.GetSSORoleCredentials(context.Background(), creds.SSOParams{
		StartURL:  startURL,
		Region:    "us-east-1",
		AccountID: "123456789012",
		RoleName:  "AdminRole",
		CacheDir:  cacheDir,
		Endpoint:  srv.URL,
	})
	if err == nil {
		t.Fatalf("cache without expiresAt was accepted (portal attempts=%d): %+v — a malformed SSO token cache must fail closed with the re-login guidance, not be treated as never-expiring", calls.Load(), got)
	}
	if calls.Load() != 0 {
		t.Fatalf("portal was called %d time(s) despite a malformed token cache", calls.Load())
	}
	if !strings.Contains(err.Error(), "expiresAt") {
		t.Fatalf("error should name the missing field for the operator: %v", err)
	}
}
