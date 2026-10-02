// SPDX-License-Identifier: MIT

package sandbox

import (
	"slices"
	"strings"
	"testing"
)

func has(env []string, name string) bool {
	return slices.ContainsFunc(env, func(kv string) bool { return strings.HasPrefix(kv, name+"=") })
}

// TestCommandNeverInherits: a child built here never gets the daemon's secrets,
// even when the caller sets no environment — os/exec's nil Env would inherit.
func TestCommandNeverInherits(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-x")
	t.Setenv("AGEZT_VAULT_PASSPHRASE", "p")
	t.Setenv("HTTPS_PROXY", "http://proxy:3128")
	for _, env := range [][]string{Command("git", "status").Env, CommandContext(t.Context(), "git").Env} {
		if env == nil {
			t.Fatal("Env is nil: the child would inherit the daemon's environment")
		}
		if has(env, "OPENAI_API_KEY") || has(env, "AGEZT_VAULT_PASSPHRASE") {
			t.Fatalf("secret reached the child env: %v", env)
		}
		if !has(env, "HTTPS_PROXY") {
			t.Fatal("proxy dropped: the child could not reach the network behind a proxy")
		}
	}
}

// TestHelperEnv keeps toolchain configuration (an allowlist would drop it) and
// drops secrets; grants are the explicit way a helper gets its own token.
func TestHelperEnv(t *testing.T) {
	t.Setenv("GOBIN", "/opt/go/bin")
	t.Setenv("ANTHROPIC_API_KEY", "sk-y")
	t.Setenv("NGROK_AUTHTOKEN", "tok")
	env := HelperEnv("NGROK_AUTHTOKEN")
	if !has(env, "GOBIN") || has(env, "ANTHROPIC_API_KEY") || !has(env, "NGROK_AUTHTOKEN") {
		t.Fatalf("HelperEnv: GOBIN=%v ANTHROPIC_API_KEY=%v NGROK_AUTHTOKEN=%v",
			has(env, "GOBIN"), has(env, "ANTHROPIC_API_KEY"), has(env, "NGROK_AUTHTOKEN"))
	}
	if !has(IsolatedEnv("NGROK_AUTHTOKEN"), "NGROK_AUTHTOKEN") || has(IsolatedEnv(), "GOBIN") {
		t.Fatal("IsolatedEnv: grants must be added, non-allowlisted names must not")
	}
}
