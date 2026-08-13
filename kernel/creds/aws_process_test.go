// SPDX-License-Identifier: MIT

package creds_test

import (
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/creds"
)

// helperProcessJSON is the fixed JSON payload our test helper
// emits — picked to be obviously synthetic so a real-world
// regression involving leaked test data is easy to spot.
const helperProcessJSON = `{"Version":1,"AccessKeyId":"AKIA-TEST-PROC","SecretAccessKey":"secret-from-process","SessionToken":"sess-tok","Expiration":"2099-01-01T00:00:00Z"}`

// helperBin builds a tiny binary that prints helperProcessJSON
// on stdout and exits. We reuse the OS's "echo" via go run is
// too slow; instead, write a single-file Go program and compile
// it once per test.
func helperBin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte(`package main
import "fmt"
func main(){ fmt.Print(`+"`"+helperProcessJSON+"`"+`) }
`), 0o600); err != nil {
		t.Fatalf("write helper src: %v", err)
	}
	name := "credproc"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(dir, name)
	// Use go build inline; this matches the pattern echoplugin and
	// mockmcp use for test fixtures elsewhere in the repo.
	out, err := runGoBuild(src, bin)
	if err != nil {
		t.Fatalf("build helper: %v\n%s", err, out)
	}
	return bin
}

// TestAWS_CredentialProcess_HappyPath: config has
// `credential_process = <path-to-binary>`, env gate enabled,
// the binary returns valid JSON → creds available via lookup.
func TestAWS_CredentialProcess_HappyPath(t *testing.T) {
	bin := helperBin(t)
	dir := t.TempDir()
	credsPath := filepath.Join(dir, "credentials")
	if err := os.WriteFile(credsPath, []byte("[default]\ncredential_process = "+bin+"\n"), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsPath)
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "no-cfg"))
	t.Setenv(creds.EnvCredentialProcessAllowed, "1")

	lookup := creds.AWSSharedCredentialsLookup("default")
	if got, want := lookup("AWS_ACCESS_KEY_ID"), "AKIA-TEST-PROC"; got != want {
		t.Errorf("AWS_ACCESS_KEY_ID = %q, want %q", got, want)
	}
	if got, want := lookup("AWS_SECRET_ACCESS_KEY"), "secret-from-process"; got != want {
		t.Errorf("AWS_SECRET_ACCESS_KEY = %q, want %q", got, want)
	}
	if got, want := lookup("AWS_SESSION_TOKEN"), "sess-tok"; got != want {
		t.Errorf("AWS_SESSION_TOKEN = %q, want %q", got, want)
	}
}

// TestAWS_CredentialProcess_GateDisabled: even with the config
// pointing at a real helper, the chain refuses to exec it when
// the operator hasn't opted in. Defaults safe.
func TestAWS_CredentialProcess_GateDisabled(t *testing.T) {
	bin := helperBin(t)
	dir := t.TempDir()
	credsPath := filepath.Join(dir, "credentials")
	_ = os.WriteFile(credsPath, []byte("[default]\ncredential_process = "+bin+"\n"), 0o600)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsPath)
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "no-cfg"))
	t.Setenv(creds.EnvCredentialProcessAllowed, "") // explicitly off

	lookup := creds.AWSSharedCredentialsLookup("default")
	if got := lookup("AWS_ACCESS_KEY_ID"); got != "" {
		t.Errorf("got %q without opt-in; expected empty", got)
	}
}

// TestAWS_CredentialProcess_InlineCredsWin: a profile with BOTH
// inline credentials and credential_process should use the
// inline values (matches AWS-SDK precedence) — no need to exec
// the binary when the answer is already in the file.
func TestAWS_CredentialProcess_InlineCredsWin(t *testing.T) {
	bin := helperBin(t)
	dir := t.TempDir()
	credsPath := filepath.Join(dir, "credentials")
	_ = os.WriteFile(credsPath, []byte(
		`[default]
aws_access_key_id = INLINE_AKID
aws_secret_access_key = INLINE_SECRET
credential_process = `+bin+`
`), 0o600)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsPath)
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "no-cfg"))
	t.Setenv(creds.EnvCredentialProcessAllowed, "1")

	lookup := creds.AWSSharedCredentialsLookup("default")
	if got, want := lookup("AWS_ACCESS_KEY_ID"), "INLINE_AKID"; got != want {
		t.Errorf("got %q, want %q (inline must win over credential_process)", got, want)
	}
}

// SEC-003: the helper's environment. cmd.Env was never set, so a credential
// helper — frequently a third-party binary named by a config file, invoked
// precisely because we do not want to hold the credential ourselves — inherited
// the daemon's ENTIRE environment: the vault passphrase, every provider key, the
// console password.
//
// The scrub cannot simply drop everything: AWS_PROFILE and friends select WHICH
// identity the helper should produce, and a helper that cannot see them serves
// the wrong profile or fails. So this asserts all three behaviours at once —
// secrets gone, selectors kept, and the operator escape hatch working.
func TestAWS_CredentialProcess_EnvIsScrubbed(t *testing.T) {
	bin, envDump := envReportingHelperBin(t)
	dir := t.TempDir()
	credsPath := filepath.Join(dir, "credentials")
	if err := os.WriteFile(credsPath, []byte("[default]\ncredential_process = "+bin+"\n"), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsPath)
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "no-cfg"))
	t.Setenv(creds.EnvCredentialProcessAllowed, "1")

	// Daemon secrets that must NOT cross the process boundary.
	t.Setenv("AGEZT_VAULT_PASSPHRASE", "vault-passphrase-must-not-leak")
	t.Setenv("OPENAI_API_KEY", "sk-must-not-leak-to-the-helper")
	t.Setenv("AGEZT_WEB_PASSWORD", "console-password-must-not-leak")
	// A non-secret selector that MUST survive, or the feature breaks.
	t.Setenv("AWS_PROFILE", "chosen-profile")
	// A helper-specific var that is NOT forwarded unless the operator says so.
	t.Setenv("VAULT_ADDR", "http://vault.internal:8200")

	if got := creds.AWSSharedCredentialsLookup("default")("AWS_ACCESS_KEY_ID"); got != "AKIA-TEST-PROC" {
		t.Fatalf("helper did not run (AWS_ACCESS_KEY_ID = %q) — the assertions below would be vacuous", got)
	}
	seen := readEnvDump(t, envDump)

	for _, leak := range []string{
		"vault-passphrase-must-not-leak",
		"sk-must-not-leak-to-the-helper",
		"console-password-must-not-leak",
	} {
		if strings.Contains(seen, leak) {
			t.Errorf("the credential helper was handed %q (SEC-003)", leak)
		}
	}
	if !strings.Contains(seen, "AWS_PROFILE=chosen-profile") {
		t.Errorf("AWS_PROFILE did not reach the helper — it cannot know which identity to produce:\n%s", seen)
	}
	if strings.Contains(seen, "vault.internal") {
		t.Error("VAULT_ADDR was forwarded without the operator asking for it")
	}

	// The escape hatch: named vars are forwarded on top of the scrubbed base.
	t.Setenv(creds.EnvCredentialProcessEnv, "VAULT_ADDR")
	if got := creds.AWSSharedCredentialsLookup("default")("AWS_ACCESS_KEY_ID"); got != "AKIA-TEST-PROC" {
		t.Fatalf("second helper run failed: %q", got)
	}
	seen = readEnvDump(t, envDump)
	if !strings.Contains(seen, "VAULT_ADDR=http://vault.internal:8200") {
		t.Errorf("%s did not forward VAULT_ADDR:\n%s", creds.EnvCredentialProcessEnv, seen)
	}
	if strings.Contains(seen, "vault-passphrase-must-not-leak") {
		t.Error("the escape hatch must not re-open the scrub for unnamed secrets")
	}
}

// ----- helpers ----------------------------------------------------

// envReportingHelperBin builds a credential_process helper that additionally
// dumps the environment it was given to a sibling file, so a test can assert on
// what actually crossed the process boundary rather than on our own bookkeeping.
// It writes to a fixed path next to itself so the credential_process config line
// stays a bare path (no argument quoting to get wrong on Windows).
func envReportingHelperBin(t *testing.T) (bin, envDump string) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	prog := `package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	_ = os.WriteFile(os.Args[0]+".env", []byte(strings.Join(os.Environ(), "\n")), 0o600)
	fmt.Print(` + "`" + helperProcessJSON + "`" + `)
}
`
	if err := os.WriteFile(src, []byte(prog), 0o600); err != nil {
		t.Fatalf("write helper src: %v", err)
	}
	name := "credprocenv"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin = filepath.Join(dir, name)
	if out, err := runGoBuild(src, bin); err != nil {
		t.Fatalf("build helper: %v\n%s", err, out)
	}
	return bin, bin + ".env"
}

func readEnvDump(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read helper env dump: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("clear helper env dump between runs: %v", err)
	}
	return string(b)
}

func runGoBuild(src, out string) ([]byte, error) {
	cmd := osexec.Command("go", "build", "-o", out, src)
	return cmd.CombinedOutput()
}
