// SPDX-License-Identifier: MIT

package creds_test

import (
	"encoding/json"
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

// TestAWSSharedCredentialsLookup_ReadsCredentialsFile verifies the
// happy path: a credentials file with one [default] section returns
// the expected key/secret/token.
func TestAWSSharedCredentialsLookup_ReadsCredentialsFile(t *testing.T) {
	dir := t.TempDir()
	credsPath := filepath.Join(dir, "credentials")
	if err := os.WriteFile(credsPath, []byte(`[default]
aws_access_key_id = AKIA111111111111
aws_secret_access_key = secret1234567890
aws_session_token = tokenABC
region = us-west-2
`), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsPath)
	// Make sure we don't accidentally pick up the operator's real
	// config file.
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "nonexistent-config"))
	t.Setenv("AWS_PROFILE", "")

	lookup := creds.AWSSharedCredentialsLookup("")
	checks := map[string]string{
		"AWS_ACCESS_KEY_ID":     "AKIA111111111111",
		"AWS_SECRET_ACCESS_KEY": "secret1234567890",
		"AWS_SESSION_TOKEN":     "tokenABC",
		"AWS_REGION":            "us-west-2",
		"AWS_DEFAULT_REGION":    "us-west-2",
	}
	for k, want := range checks {
		if got := lookup(k); got != want {
			t.Errorf("lookup(%q) = %q, want %q", k, got, want)
		}
	}
}

// TestAWSSharedCredentialsLookup_HonoursProfile verifies that an
// explicit profile arg (or AWS_PROFILE) picks the right section.
func TestAWSSharedCredentialsLookup_HonoursProfile(t *testing.T) {
	dir := t.TempDir()
	credsPath := filepath.Join(dir, "credentials")
	if err := os.WriteFile(credsPath, []byte(`[default]
aws_access_key_id = AKIA_DEFAULT
aws_secret_access_key = secret_default

[work]
aws_access_key_id = AKIA_WORK
aws_secret_access_key = secret_work
`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsPath)
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "no-config"))

	lookup := creds.AWSSharedCredentialsLookup("work")
	if got, want := lookup("AWS_ACCESS_KEY_ID"), "AKIA_WORK"; got != want {
		t.Errorf("AWS_ACCESS_KEY_ID = %q, want %q", got, want)
	}

	// AWS_PROFILE env var (when profile arg is empty).
	t.Setenv("AWS_PROFILE", "work")
	lookup2 := creds.AWSSharedCredentialsLookup("")
	if got, want := lookup2("AWS_ACCESS_KEY_ID"), "AKIA_WORK"; got != want {
		t.Errorf("AWS_PROFILE-selected AWS_ACCESS_KEY_ID = %q, want %q", got, want)
	}
}

// TestAWSSharedCredentialsLookup_ConfigFileSuppliesRegion verifies
// that region in ~/.aws/config (which uses `[profile NAME]` rather
// than `[NAME]`) is picked up when credentials file omits it.
func TestAWSSharedCredentialsLookup_ConfigFileSuppliesRegion(t *testing.T) {
	dir := t.TempDir()
	credsPath := filepath.Join(dir, "credentials")
	if err := os.WriteFile(credsPath, []byte(`[work]
aws_access_key_id = AKIA_WORK
aws_secret_access_key = secret_work
`), 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	cfgPath := filepath.Join(dir, "config")
	if err := os.WriteFile(cfgPath, []byte(`[profile work]
region = eu-west-1
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsPath)
	t.Setenv("AWS_CONFIG_FILE", cfgPath)

	lookup := creds.AWSSharedCredentialsLookup("work")
	if got, want := lookup("AWS_REGION"), "eu-west-1"; got != want {
		t.Errorf("AWS_REGION = %q, want %q (must read from config file, not credentials)", got, want)
	}
	// AKID still from credentials.
	if got, want := lookup("AWS_ACCESS_KEY_ID"), "AKIA_WORK"; got != want {
		t.Errorf("AWS_ACCESS_KEY_ID = %q, want %q", got, want)
	}
}

// TestAWSSharedCredentialsLookup_DefaultProfileNoPrefix verifies the
// AWS-CLI quirk where ~/.aws/config's default section is bare
// `[default]` while named profiles use `[profile X]`. Failing to
// account for this would make every "I'm using the default profile,
// why isn't my region picked up?" case silently fail.
func TestAWSSharedCredentialsLookup_DefaultProfileNoPrefix(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	if err := os.WriteFile(cfgPath, []byte(`[default]
region = ap-southeast-2
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "no-creds"))
	t.Setenv("AWS_CONFIG_FILE", cfgPath)
	t.Setenv("AWS_PROFILE", "")

	lookup := creds.AWSSharedCredentialsLookup("default")
	if got, want := lookup("AWS_REGION"), "ap-southeast-2"; got != want {
		t.Errorf("default-profile AWS_REGION = %q, want %q", got, want)
	}
}

// TestAWSSharedCredentialsLookup_MissingFile returns empty without
// panicking. The chain falls through silently in production.
func TestAWSSharedCredentialsLookup_MissingFile(t *testing.T) {
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "nope"))
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "nope2"))
	lookup := creds.AWSSharedCredentialsLookup("default")
	if got := lookup("AWS_ACCESS_KEY_ID"); got != "" {
		t.Errorf("got %q from missing file, want empty", got)
	}
}

// TestAWSSharedCredentialsLookup_RecognisedNamesOnly verifies the
// lookup returns empty for non-AWS names so the lookup composes
// cleanly with ChainLookup (the chain falls through for unmatched
// names).
func TestAWSSharedCredentialsLookup_RecognisedNamesOnly(t *testing.T) {
	dir := t.TempDir()
	credsPath := filepath.Join(dir, "credentials")
	_ = os.WriteFile(credsPath, []byte(`[default]
aws_access_key_id = X
aws_secret_access_key = Y
`), 0o600)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsPath)
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "no-cfg"))

	lookup := creds.AWSSharedCredentialsLookup("default")
	for _, name := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "WHATEVER"} {
		if got := lookup(name); got != "" {
			t.Errorf("lookup(%q) = %q, want empty (non-AWS name)", name, got)
		}
	}
}

// fakeIMDS sets up an httptest.Server that mimics the IMDSv2 flow:
// PUT /latest/api/token → returns a token, GET endpoints reject
// requests missing the token header.
type fakeIMDS struct {
	server      *httptest.Server
	tokenReqs   atomic.Int64
	credReqs    atomic.Int64
	credPayload []byte
}

func newFakeIMDS(t *testing.T, role string, akid, secret, sessionToken, region string, expiresIn time.Duration) *fakeIMDS {
	t.Helper()
	f := &fakeIMDS{}
	credResp := map[string]any{
		"Code":            "Success",
		"AccessKeyId":     akid,
		"SecretAccessKey": secret,
		"Token":           sessionToken,
		"Expiration":      time.Now().Add(expiresIn).UTC().Format(time.RFC3339),
	}
	f.credPayload, _ = json.Marshal(credResp)

	mux := http.NewServeMux()
	mux.HandleFunc("/latest/api/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		f.tokenReqs.Add(1)
		fmt.Fprint(w, "TOKEN-XYZ")
	})
	mux.HandleFunc("/latest/meta-data/iam/security-credentials/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-aws-ec2-metadata-token") != "TOKEN-XYZ" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// Trailing slash → return role name.
		// /<role> → return credentials JSON.
		if r.URL.Path == "/latest/meta-data/iam/security-credentials/" {
			fmt.Fprint(w, role)
			return
		}
		if r.URL.Path == "/latest/meta-data/iam/security-credentials/"+role {
			f.credReqs.Add(1)
			w.Write(f.credPayload)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/latest/meta-data/placement/region", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-aws-ec2-metadata-token") != "TOKEN-XYZ" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, region)
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

// TestAWSIMDSLookup_HappyPath verifies a successful IMDSv2 handshake
// + role-credentials fetch + region fetch returns the expected
// values via the lookup.
func TestAWSIMDSLookup_HappyPath(t *testing.T) {
	fake := newFakeIMDS(t, "demo-role", "AKIAIMDS", "imds-secret", "imds-token", "us-east-2", 30*time.Minute)
	t.Setenv("AWS_EC2_METADATA_BASE", fake.server.URL)

	lookup := creds.AWSIMDSLookup(&http.Client{Timeout: 2 * time.Second})
	if got, want := lookup("AWS_ACCESS_KEY_ID"), "AKIAIMDS"; got != want {
		t.Errorf("AWS_ACCESS_KEY_ID = %q, want %q", got, want)
	}
	if got, want := lookup("AWS_SECRET_ACCESS_KEY"), "imds-secret"; got != want {
		t.Errorf("AWS_SECRET_ACCESS_KEY = %q, want %q", got, want)
	}
	if got, want := lookup("AWS_SESSION_TOKEN"), "imds-token"; got != want {
		t.Errorf("AWS_SESSION_TOKEN = %q, want %q", got, want)
	}
	if got, want := lookup("AWS_REGION"), "us-east-2"; got != want {
		t.Errorf("AWS_REGION = %q, want %q", got, want)
	}
}

// TestAWSIMDSLookup_CachesAcrossCalls verifies that multiple
// lookups against the same source share a single IMDS handshake —
// we don't re-handshake per lookup.
func TestAWSIMDSLookup_CachesAcrossCalls(t *testing.T) {
	fake := newFakeIMDS(t, "demo", "X", "Y", "Z", "us-east-1", 30*time.Minute)
	t.Setenv("AWS_EC2_METADATA_BASE", fake.server.URL)
	lookup := creds.AWSIMDSLookup(&http.Client{Timeout: 2 * time.Second})

	// Prime by reading several values; each should reuse the cache.
	for _, k := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION", "AWS_ACCESS_KEY_ID"} {
		lookup(k)
	}
	if got := fake.credReqs.Load(); got != 1 {
		t.Errorf("cred fetch count = %d, want 1 (cache should suppress repeats)", got)
	}
}

// TestAWSIMDSLookup_FailureFallsThroughAndNegCaches verifies that
// IMDS unreachable → returns empty, AND further lookups within the
// neg-cache window don't retry.
func TestAWSIMDSLookup_FailureFallsThroughAndNegCaches(t *testing.T) {
	// Server that always returns 500 on the token endpoint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "imds dead", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("AWS_EC2_METADATA_BASE", srv.URL)

	reqs := atomic.Int64{}
	client := &http.Client{
		Timeout: 1 * time.Second,
		Transport: countingRoundTripper{
			inner:   http.DefaultTransport,
			counter: &reqs,
		},
	}
	lookup := creds.AWSIMDSLookup(client)
	if got := lookup("AWS_ACCESS_KEY_ID"); got != "" {
		t.Errorf("got %q, want empty (imds failed)", got)
	}
	first := reqs.Load()
	// Second lookup within neg-cache window should NOT hit the server.
	if got := lookup("AWS_SECRET_ACCESS_KEY"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
	if reqs.Load() != first {
		t.Errorf("neg-cache leaked: req count went from %d to %d", first, reqs.Load())
	}
}

// TestAWSIMDSLookup_NonAWSNameReturnsEmpty mirrors the shared-
// credentials test — IMDS lookup must filter out non-AWS names too.
func TestAWSIMDSLookup_NonAWSNameReturnsEmpty(t *testing.T) {
	// Should not even need to hit IMDS for the filter to bite.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("IMDS hit for non-AWS name lookup; URL=%s", r.URL)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("AWS_EC2_METADATA_BASE", srv.URL)
	lookup := creds.AWSIMDSLookup(&http.Client{Timeout: 1 * time.Second})
	if got := lookup("OPENAI_API_KEY"); got != "" {
		t.Errorf("got %q, want empty (non-AWS name)", got)
	}
}

// TestAWSDefaultChain_EnvWins verifies the composed default chain
// honours AWS SDK precedence (env over file over IMDS).
func TestAWSDefaultChain_EnvWins(t *testing.T) {
	dir := t.TempDir()
	credsPath := filepath.Join(dir, "credentials")
	_ = os.WriteFile(credsPath, []byte(`[default]
aws_access_key_id = FROM_FILE
aws_secret_access_key = SECRET_FILE
`), 0o600)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsPath)
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "no-cfg"))
	t.Setenv("AWS_ACCESS_KEY_ID", "FROM_ENV")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET_ENV")
	// Point IMDS at a server that'll fail to avoid timing dependence.
	t.Setenv("AWS_EC2_METADATA_BASE", "http://127.0.0.1:1")

	chain := creds.AWSDefaultChain()
	if got, want := chain("AWS_ACCESS_KEY_ID"), "FROM_ENV"; got != want {
		t.Errorf("AWS_ACCESS_KEY_ID = %q, want %q (env should beat file)", got, want)
	}
}

// TestAWSDefaultChain_FallsThroughToFile verifies that when env is
// empty the chain reads from ~/.aws/credentials.
func TestAWSDefaultChain_FallsThroughToFile(t *testing.T) {
	dir := t.TempDir()
	credsPath := filepath.Join(dir, "credentials")
	_ = os.WriteFile(credsPath, []byte(`[default]
aws_access_key_id = FILE_KEY
aws_secret_access_key = FILE_SECRET
region = us-east-1
`), 0o600)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsPath)
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "no-cfg"))
	// Clear any inherited env.
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_EC2_METADATA_BASE", "http://127.0.0.1:1")

	chain := creds.AWSDefaultChain()
	if got, want := chain("AWS_ACCESS_KEY_ID"), "FILE_KEY"; got != want {
		t.Errorf("AWS_ACCESS_KEY_ID = %q, want %q", got, want)
	}
	if got, want := chain("AWS_REGION"), "us-east-1"; got != want {
		t.Errorf("AWS_REGION = %q, want %q", got, want)
	}
}

// ----- helpers ---------------------------------------------------

type countingRoundTripper struct {
	inner   http.RoundTripper
	counter *atomic.Int64
}

func (r countingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.counter.Add(1)
	return r.inner.RoundTrip(req)
}
