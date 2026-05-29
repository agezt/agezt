// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestConfig_ReturnsResolvedPathsAndCounts asserts the wire fields
// the `agt config show` CLI relies on. paths.base should match the
// kernel's BaseDir, tool_count = 1 (only "shell" registered via
// startPair), ask_policy = "allow" (engine default), env should
// be a (possibly empty) map.
func TestConfig_ReturnsResolvedPathsAndCounts(t *testing.T) {
	_, _, c, dir := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdConfig, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	paths, _ := res["paths"].(map[string]any)
	if paths == nil {
		t.Fatalf("paths missing; got %v", res)
	}
	if got, _ := paths["base"].(string); got != dir {
		t.Errorf("paths.base = %q want %q", got, dir)
	}
	wantJournal := filepath.Join(dir, "journal")
	if got, _ := paths["journal"].(string); got != wantJournal {
		t.Errorf("paths.journal = %q want %q", got, wantJournal)
	}
	for _, k := range []string{"state", "runtime", "catalog", "vault"} {
		if v, _ := paths[k].(string); !strings.HasPrefix(v, dir) {
			t.Errorf("paths.%s = %q; should be under base dir %q", k, v, dir)
		}
	}

	if got := intOf(res["tool_count"]); got != 1 {
		t.Errorf("tool_count = %d want 1", got)
	}
	if got := intOf(res["plugin_count"]); got != 0 {
		t.Errorf("plugin_count = %d want 0", got)
	}
	if got, _ := res["ask_policy"].(string); got != "allow" {
		t.Errorf("ask_policy = %q want %q (engine default)", got, "allow")
	}
	if _, ok := res["system_prompt_set"].(bool); !ok {
		t.Errorf("system_prompt_set missing or wrong type; got %v", res["system_prompt_set"])
	}
	if _, ok := res["env"].(map[string]any); !ok {
		t.Errorf("env missing or wrong type; got %v", res["env"])
	}
}

// TestConfig_EnvPresenceIsBooleanNotValue is the privacy contract:
// when AGEZT_VAULT_PASSPHRASE is set, the response should include
// the key with value `true` — NOT the passphrase itself. This is
// load-bearing for "paste the config output into a bug report".
func TestConfig_EnvPresenceIsBooleanNotValue(t *testing.T) {
	t.Setenv("AGEZT_VAULT_PASSPHRASE", "super-secret-do-not-leak")
	t.Setenv("AGEZT_MODEL", "test-model-id")

	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Call(context.Background(), controlplane.CmdConfig, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	env, _ := res["env"].(map[string]any)
	if env == nil {
		t.Fatalf("env missing; got %v", res)
	}
	if env["AGEZT_VAULT_PASSPHRASE"] != true {
		t.Errorf("AGEZT_VAULT_PASSPHRASE = %v; want true (presence indicator)", env["AGEZT_VAULT_PASSPHRASE"])
	}
	if env["AGEZT_MODEL"] != true {
		t.Errorf("AGEZT_MODEL = %v; want true", env["AGEZT_MODEL"])
	}
	// Verify the passphrase value itself is not echoed anywhere in
	// the response. Marshal once and grep.
	for k, v := range env {
		if s, ok := v.(string); ok && strings.Contains(s, "super-secret") {
			t.Errorf("env[%q] leaked passphrase value: %q", k, s)
		}
	}
}

// TestConfig_SystemPromptPresenceOnly mirrors the privacy contract
// for the system prompt. The kernel was opened without one in
// startPair, so system_prompt_set should be false. Content is
// never sent.
func TestConfig_SystemPromptPresenceOnly(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Call(context.Background(), controlplane.CmdConfig, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if v, _ := res["system_prompt_set"].(bool); v {
		t.Errorf("system_prompt_set = true; want false (startPair sets no prompt)")
	}
	// There must be no top-level "system_prompt" string key — only
	// the boolean presence indicator.
	if _, ok := res["system_prompt"]; ok {
		t.Errorf("system_prompt key present in response; only system_prompt_set is permitted")
	}
}
