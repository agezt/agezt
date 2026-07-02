// SPDX-License-Identifier: MIT

package executionprofile

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/warden"
)

func TestEnvPassthroughPolicyFromEnv(t *testing.T) {
	t.Setenv(EnvDocker, "SAFE_VISIBLE, OPENAI_API_KEY, SAFE_VISIBLE, bad-name")
	t.Setenv(SecretEnvDocker, "OPENAI_API_KEY, AGEZT_WEB_PASSWORD, GITHUB_TOKEN")

	policy := EnvPassthroughPolicyFromEnv("docker")
	if got := strings.Join(policy.EnvNames, ","); got != "SAFE_VISIBLE" {
		t.Fatalf("EnvNames = %q, want SAFE_VISIBLE", got)
	}
	if got := strings.Join(policy.SecretEnvNames, ","); got != "GITHUB_TOKEN,OPENAI_API_KEY" {
		t.Fatalf("SecretEnvNames = %q, want sorted non-AGEZT secret names", got)
	}
}

func TestEnvPassthroughResolve(t *testing.T) {
	policy := EnvPassthroughPolicy{
		Profile:        "docker",
		EnvNames:       []string{"SAFE_VISIBLE", "OPENAI_API_KEY"},
		SecretEnvNames: []string{"OPENAI_API_KEY", "AGEZT_WEB_PASSWORD", "GITHUB_TOKEN"},
	}
	values := map[string]string{
		"SAFE_VISIBLE":       "ok",
		"OPENAI_API_KEY":     "sk-test",
		"AGEZT_WEB_PASSWORD": "internal",
		"GITHUB_TOKEN":       "ghp-test",
	}
	env := policy.Resolve(func(name string) (string, bool) {
		v, ok := values[name]
		return v, ok
	})
	joined := strings.Join(env, "\n")
	for _, want := range []string{"SAFE_VISIBLE=ok", "OPENAI_API_KEY=sk-test", "GITHUB_TOKEN=ghp-test"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("resolved env missing %q:\n%s", want, joined)
		}
	}
	for _, leak := range []string{"AGEZT_WEB_PASSWORD", "internal"} {
		if strings.Contains(joined, leak) {
			t.Fatalf("resolved env leaked %q:\n%s", leak, joined)
		}
	}
}

func TestAppendEnvPassthroughMergesByName(t *testing.T) {
	t.Setenv("SAFE_VISIBLE", "ok")
	t.Setenv(EnvLocal, "SAFE_VISIBLE,PATH")
	t.Setenv("PATH", "/custom/bin")

	env := AppendEnvPassthrough([]string{"PATH=/base/bin", "HOME=/tmp/work"}, IDLocal)
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "PATH=/base/bin") || !strings.Contains(joined, "PATH=/custom/bin") {
		t.Fatalf("PATH should be replaced by passthrough value:\n%s", joined)
	}
	if !strings.Contains(joined, "SAFE_VISIBLE=ok") {
		t.Fatalf("SAFE_VISIBLE missing:\n%s", joined)
	}
}

func TestProfileIDForWardenProfile(t *testing.T) {
	cases := map[warden.Profile]string{
		warden.ProfileNone:      IDLocal,
		warden.ProfileNamespace: IDWarden,
		warden.ProfileContainer: "docker",
	}
	for in, want := range cases {
		if got := ProfileIDForWardenProfile(in); got != want {
			t.Fatalf("ProfileIDForWardenProfile(%q) = %q, want %q", in, got, want)
		}
	}
}
