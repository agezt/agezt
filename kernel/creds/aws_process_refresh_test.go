// SPDX-License-Identifier: MIT

package creds_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/creds"
)

// countingHelperSrc is a credential_process helper that mints a FRESH
// credential on every invocation: it increments a counter file (argv[1]),
// embeds the count in the key material, and prints the AWS spec JSON.
// argv[2] selects whether the payload carries an `Expiration` two hours IN
// THE PAST — so after the first mint the credential is already stale and a
// refresh requires re-running the helper (no sleeps, fully deterministic).
const countingHelperSrc = `package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	counter := os.Args[1]
	withExpiry := os.Args[2] == "EXPIRING"
	n := 0
	if b, err := os.ReadFile(counter); err == nil {
		n, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	n++
	_ = os.WriteFile(counter, []byte(strconv.Itoa(n)), 0o600)
	id := "AKIDCALL-" + strconv.Itoa(n)
	secret := "SECRET-" + strconv.Itoa(n)
	tok := "TOK-" + strconv.Itoa(n)
	out := "{\"Version\":1,\"AccessKeyId\":\"" + id + "\",\"SecretAccessKey\":\"" + secret + "\",\"SessionToken\":\"" + tok + "\""
	if withExpiry {
		out += ",\"Expiration\":\"" + time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339) + "\""
	}
	fmt.Print(out + "}")
}
`

// countingHelperBin builds the counting helper and returns its path plus the
// counter file the test can inspect to learn exactly how many times the
// helper ran. The mode string is passed through to the helper as argv[2]:
// "EXPIRING" mints credentials whose Expiration is two hours in the past,
// "PERMANENT" mints credentials with no Expiration at all.
func countingHelperBin(t *testing.T, mode string) (bin, counter string) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte(countingHelperSrc), 0o600); err != nil {
		t.Fatalf("write helper src: %v", err)
	}
	name := "credproccount"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binPath := filepath.Join(dir, name)
	if out, err := runGoBuild(src, binPath); err != nil {
		t.Fatalf("build helper: %v\n%s", err, out)
	}
	return binPath, filepath.Join(dir, "counter")
}

// countingHelperLookup wires a profile whose credential_process points at the
// counting helper and returns the lookup plus the counter path.
func countingHelperLookup(t *testing.T, mode string) (lookup func(string) string, counter string) {
	t.Helper()
	bin, counter := countingHelperBin(t, mode)
	dir := t.TempDir()
	credsPath := filepath.Join(dir, "credentials")
	if err := os.WriteFile(credsPath, []byte("[default]\ncredential_process = "+bin+" "+counter+" "+mode+"\n"), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsPath)
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "no-cfg"))
	t.Setenv(creds.EnvCredentialProcessAllowed, "1")
	return creds.AWSSharedCredentialsLookup("default"), counter
}

// helperInvocations reads the counter file the helper maintains.
func helperInvocations(t *testing.T, counter string) string {
	t.Helper()
	b, err := os.ReadFile(counter)
	if err != nil {
		t.Fatalf("read counter: %v", err)
	}
	return strings.TrimSpace(string(b))
}

// TestAWS_CredentialProcess_ExpiringCredsRefresh: a credential_process helper
// that mints TEMPORARY credentials (SessionToken + Expiration — the normal
// shape for aws-vault / 1Password-style helpers) must be RE-INVOKED once the
// minted credentials pass their Expiration, exactly like the IMDS and STS
// sources. Regression guard: the helper used to run exactly once per process
// (the whole shared-files load sat behind a sync.Once) and Expiration was
// never parsed, so after the minted session expired every AWS call failed
// with expired-token errors until daemon restart.
func TestAWS_CredentialProcess_ExpiringCredsRefresh(t *testing.T) {
	lookup, counter := countingHelperLookup(t, "EXPIRING")

	if got := lookup("AWS_ACCESS_KEY_ID"); got != "AKIDCALL-1" {
		t.Fatalf("first lookup = %q, want AKIDCALL-1", got)
	}
	// The minted credential is already past its Expiration, so the very next
	// lookup must re-mint rather than serve the dead session.
	if got := lookup("AWS_ACCESS_KEY_ID"); got != "AKIDCALL-2" {
		t.Errorf("second lookup = %q, want a re-minted AKIDCALL-2: the credential had "+
			"already expired, but the helper was not re-invoked (helper ran %s time(s))",
			got, helperInvocations(t, counter))
	}
	// Every mint from this helper is already expired, so each subsequent
	// lookup re-mints too — the token must therefore come from mint 3, and
	// the counter must show one helper run per lookup (never a served dead
	// session).
	if got := lookup("AWS_SESSION_TOKEN"); got != "TOK-3" {
		t.Errorf("session token after two expired mints = %q, want the freshly minted TOK-3", got)
	}
	if n := helperInvocations(t, counter); n != "3" {
		t.Errorf("helper invoked %s time(s) after three lookups on always-expired credentials, want 3", n)
	}
}

// TestAWS_CredentialProcess_PermanentCredsRunOnce: credentials minted WITHOUT
// an Expiration are permanent — the helper must run exactly once no matter
// how many lookups follow (refreshing them would exec a process on every
// cache miss for no benefit).
func TestAWS_CredentialProcess_PermanentCredsRunOnce(t *testing.T) {
	lookup, counter := countingHelperLookup(t, "PERMANENT")

	for i := 1; i <= 3; i++ {
		if got := lookup("AWS_ACCESS_KEY_ID"); got != "AKIDCALL-1" {
			t.Errorf("lookup %d = %q, want AKIDCALL-1", i, got)
		}
	}
	if n := helperInvocations(t, counter); n != "1" {
		t.Errorf("helper invoked %s time(s) for credentials with no Expiration, want exactly 1", n)
	}
}
