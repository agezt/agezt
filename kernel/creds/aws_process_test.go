// SPDX-License-Identifier: MIT

package creds_test

import (
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ersinkoc/agezt/kernel/creds"
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

// ----- helpers ----------------------------------------------------

func runGoBuild(src, out string) ([]byte, error) {
	cmd := osexec.Command("go", "build", "-o", out, src)
	return cmd.CombinedOutput()
}
