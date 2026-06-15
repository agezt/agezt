// SPDX-License-Identifier: MIT

package shell

import (
	"strings"
	"testing"
)

func TestIsSecretName(t *testing.T) {
	secret := []string{"OPENAI_API_KEY", "GITHUB_TOKEN", "DB_PASSWORD", "MY_SECRET", "AWS_ACCESS_KEY_ID", "AGEZT_WEB_PASSWORD", "SOME_CRED"}
	for _, n := range secret {
		if !isSecretName(strings.ToUpper(n)) {
			t.Errorf("%s should be flagged secret", n)
		}
	}
	safe := []string{"PATH", "SYSTEMROOT", "COMSPEC", "LANG", "USERPROFILE", "NUMBER_OF_PROCESSORS"}
	for _, n := range safe {
		if isSecretName(strings.ToUpper(n)) {
			t.Errorf("%s should NOT be flagged secret", n)
		}
	}
}

func TestScrubEnv_KeepsPathDropsSecrets(t *testing.T) {
	// A real PATH must already exist in the test process env.
	t.Setenv("PATH", t.TempDir())
	// Plant secret-shaped vars that must NOT survive into the child env.
	t.Setenv("OPENAI_API_KEY", "sk-should-not-leak")
	t.Setenv("AGEZT_VAULT_PASSPHRASE", "nope")
	t.Setenv("MY_TOKEN", "leak-me-not")

	env := scrubEnv("C:\\work")
	joined := strings.Join(env, "\n")

	// PATH survives (so external programs resolve — the whole point of the fix).
	hasPath := false
	for _, kv := range env {
		if strings.HasPrefix(strings.ToUpper(kv), "PATH=") {
			hasPath = true
		}
	}
	if !hasPath {
		t.Error("scrubEnv must keep PATH")
	}
	// Secrets are gone.
	for _, leak := range []string{"OPENAI_API_KEY", "AGEZT_VAULT_PASSPHRASE", "MY_TOKEN", "sk-should-not-leak", "nope", "leak-me-not"} {
		if strings.Contains(joined, leak) {
			t.Errorf("scrubbed env leaked %q", leak)
		}
	}
	// HOME/TMP point at the work dir.
	for _, want := range []string{"HOME=C:\\work", "TEMP=C:\\work", "TMP=C:\\work", "TMPDIR=C:\\work"} {
		if !strings.Contains(joined, want) {
			t.Errorf("scrubEnv missing %q", want)
		}
	}
}

func TestScrubEnv_EmptyDirFallsBackToTemp(t *testing.T) {
	env := scrubEnv("")
	var home string
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "HOME="); ok {
			home = v
		}
	}
	if home == "" {
		t.Error("empty dir should fall back to a temp dir for HOME, got empty")
	}
}
