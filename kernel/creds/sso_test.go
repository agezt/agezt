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
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/creds"
)

func writeSSOToken(t *testing.T, cacheDir, startURL string, token string, expires time.Time) {
	t.Helper()
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sum := sha1.Sum([]byte(startURL))
	path := filepath.Join(cacheDir, hex.EncodeToString(sum[:])+".json")
	body := fmt.Sprintf(`{"startUrl":"%s","region":"us-east-1","accessToken":"%s","expiresAt":"%s"}`,
		startURL, token, expires.UTC().Format(time.RFC3339))
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
}

func TestGetSSORoleCredentials_HappyPath(t *testing.T) {
	cacheDir := t.TempDir()
	startURL := "https://acme-corp.awsapps.com/start"
	writeSSOToken(t, cacheDir, startURL, "test-access-token-xyz",
		time.Date(2026, 5, 29, 13, 0, 0, 0, time.UTC))

	var seenAuth, seenAccount, seenRole string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("x-amz-sso_bearer_token")
		seenAccount = r.URL.Query().Get("account_id")
		seenRole = r.URL.Query().Get("role_name")
		// expiration must be Unix MILLISECONDS (this is the bit that
		// surprises people coming from STS).
		expMs := time.Date(2026, 5, 29, 13, 30, 0, 0, time.UTC).UnixMilli()
		fmt.Fprintf(w, `{"roleCredentials":{"accessKeyId":"ASIA-SSO","secretAccessKey":"sk-sso","sessionToken":"tok-sso","expiration":%d}}`, expMs)
	}))
	defer srv.Close()

	got, err := creds.GetSSORoleCredentials(context.Background(), creds.SSOParams{
		StartURL:  startURL,
		Region:    "us-east-1",
		AccountID: "123456789012",
		RoleName:  "AdminRole",
		CacheDir:  cacheDir,
		Endpoint:  srv.URL,
		Now:       func() time.Time { return time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("GetSSORoleCredentials: %v", err)
	}
	if seenAuth != "test-access-token-xyz" {
		t.Errorf("bearer token = %q", seenAuth)
	}
	if seenAccount != "123456789012" || seenRole != "AdminRole" {
		t.Errorf("account/role = %q / %q", seenAccount, seenRole)
	}
	if got.Creds.AccessKeyID != "ASIA-SSO" || got.Creds.SessionToken != "tok-sso" {
		t.Errorf("creds = %+v", got.Creds)
	}
	wantExp := time.Date(2026, 5, 29, 13, 30, 0, 0, time.UTC)
	if !got.Expiration.Equal(wantExp) {
		t.Errorf("Expiration=%v want %v", got.Expiration, wantExp)
	}
}

// TestGetSSORoleCredentials_EscapesQueryParams pins M466: a role name with
// characters that are special in a URL query (a legitimate IAM role name) must
// reach the SSO portal intact. Raw concatenation would send "+" as a space and let
// "&"/"=" corrupt the query.
func TestGetSSORoleCredentials_EscapesQueryParams(t *testing.T) {
	cacheDir := t.TempDir()
	startURL := "https://acme-corp.awsapps.com/start"
	writeSSOToken(t, cacheDir, startURL, "tok",
		time.Date(2026, 5, 29, 13, 0, 0, 0, time.UTC))

	const roleName = "My+Admin@2,Role"
	const accountID = "000111222333"
	var seenAccount, seenRole string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAccount = r.URL.Query().Get("account_id")
		seenRole = r.URL.Query().Get("role_name")
		expMs := time.Date(2026, 5, 29, 13, 30, 0, 0, time.UTC).UnixMilli()
		fmt.Fprintf(w, `{"roleCredentials":{"accessKeyId":"A","secretAccessKey":"s","sessionToken":"t","expiration":%d}}`, expMs)
	}))
	defer srv.Close()

	_, err := creds.GetSSORoleCredentials(context.Background(), creds.SSOParams{
		StartURL:  startURL,
		Region:    "us-east-1",
		AccountID: accountID,
		RoleName:  roleName,
		CacheDir:  cacheDir,
		Endpoint:  srv.URL,
		Now:       func() time.Time { return time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("GetSSORoleCredentials: %v", err)
	}
	if seenRole != roleName {
		t.Errorf("role_name reached the portal as %q, want %q (query not escaped)", seenRole, roleName)
	}
	if seenAccount != accountID {
		t.Errorf("account_id = %q, want %q", seenAccount, accountID)
	}
}

func TestGetSSORoleCredentials_RejectsExpiredToken(t *testing.T) {
	cacheDir := t.TempDir()
	startURL := "https://x.awsapps.com/start"
	// Token expired one minute before "now".
	writeSSOToken(t, cacheDir, startURL, "tok",
		time.Date(2026, 5, 29, 11, 59, 0, 0, time.UTC))
	_, err := creds.GetSSORoleCredentials(context.Background(), creds.SSOParams{
		StartURL:  startURL,
		Region:    "us-east-1",
		AccountID: "1",
		RoleName:  "r",
		CacheDir:  cacheDir,
		Now:       func() time.Time { return time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC) },
	})
	if err == nil {
		t.Fatal("expected error for expired SSO token")
	}
}

func TestGetSSORoleCredentials_MissingCacheFile(t *testing.T) {
	_, err := creds.GetSSORoleCredentials(context.Background(), creds.SSOParams{
		StartURL:  "https://never-logged-in.example/start",
		Region:    "us-east-1",
		AccountID: "1",
		RoleName:  "r",
		CacheDir:  t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for missing cache file")
	}
}

func TestAWSSSOLookup_CachesUntilExpiry(t *testing.T) {
	cacheDir := t.TempDir()
	startURL := "https://corp.awsapps.com/start"
	writeSSOToken(t, cacheDir, startURL, "tok",
		time.Date(2026, 5, 29, 13, 0, 0, 0, time.UTC))

	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		expMs := time.Date(2026, 5, 29, 14, 0, 0, 0, time.UTC).UnixMilli()
		fmt.Fprintf(w, `{"roleCredentials":{"accessKeyId":"AKID","secretAccessKey":"SK","sessionToken":"TOK","expiration":%d}}`, expMs)
	}))
	defer srv.Close()

	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	lookup := creds.AWSSSOLookup(creds.SSOParams{
		StartURL:  startURL,
		Region:    "us-east-1",
		AccountID: "1",
		RoleName:  "r",
		CacheDir:  cacheDir,
		Endpoint:  srv.URL,
		Now:       func() time.Time { return now },
	})

	// 10 probes — only one HTTP call.
	for range 10 {
		_ = lookup("AWS_ACCESS_KEY_ID")
		_ = lookup("AWS_SECRET_ACCESS_KEY")
		_ = lookup("AWS_SESSION_TOKEN")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("SSO portal called %d times, want 1", n)
	}
	if lookup("AWS_REGION") != "us-east-1" {
		t.Error("region passthrough broken")
	}
	if lookup("SOMETHING") != "" {
		t.Error("unrelated name should return empty")
	}
}

func TestLoadSSOParamsFromProfile_OldLayout(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	if err := os.WriteFile(cfgPath, []byte(`[profile myprof]
sso_start_url = https://acme.awsapps.com/start
sso_region = us-east-1
sso_account_id = 123456789012
sso_role_name = AdminRole
`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("AWS_CONFIG_FILE", cfgPath)
	p, ok := creds.LoadSSOParamsFromProfile("myprof")
	if !ok {
		t.Fatal("expected SSO params recognised")
	}
	if p.StartURL != "https://acme.awsapps.com/start" || p.AccountID != "123456789012" {
		t.Errorf("params = %+v", p)
	}
}

func TestLoadSSOParamsFromProfile_NewLayoutWithSession(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	if err := os.WriteFile(cfgPath, []byte(`[profile myprof]
sso_session = corp-sso
sso_account_id = 123456789012
sso_role_name = ReadOnlyRole
[sso-session corp-sso]
sso_start_url = https://corp.awsapps.com/start
sso_region = eu-west-1
`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("AWS_CONFIG_FILE", cfgPath)
	p, ok := creds.LoadSSOParamsFromProfile("myprof")
	if !ok {
		t.Fatal("expected SSO params recognised via sso-session dereference")
	}
	if p.StartURL != "https://corp.awsapps.com/start" || p.Region != "eu-west-1" {
		t.Errorf("dereference failed: %+v", p)
	}
	if p.RoleName != "ReadOnlyRole" {
		t.Errorf("role = %q", p.RoleName)
	}
}

func TestLoadSSOParamsFromProfile_NoSSOFieldsReturnsNotOK(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	if err := os.WriteFile(cfgPath, []byte(`[profile plain]
region = us-east-1
aws_access_key_id = AKID
aws_secret_access_key = SK
`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("AWS_CONFIG_FILE", cfgPath)
	if _, ok := creds.LoadSSOParamsFromProfile("plain"); ok {
		t.Error("non-SSO profile should return ok=false")
	}
}
