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

func TestConfigSchema_ReturnsReloadBoundaries(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdConfigSchema, nil)
	if err != nil {
		t.Fatalf("config_schema: %v", err)
	}
	if sections, _ := res["sections"].([]any); len(sections) == 0 {
		t.Fatalf("sections missing from config_schema: %+v", res)
	}
	boundaries, _ := res["reload_boundaries"].([]any)
	if len(boundaries) < 2 {
		t.Fatalf("reload_boundaries missing/incomplete: %+v", res["reload_boundaries"])
	}
	byApply := map[string][]any{}
	for _, raw := range boundaries {
		row, _ := raw.(map[string]any)
		apply, _ := row["apply"].(string)
		envs, _ := row["envs"].([]any)
		byApply[apply] = envs
	}
	if !containsAnyString(byApply["live"], "AGEZT_PROVIDER") || !containsAnyString(byApply["live"], "AGEZT_MODEL") {
		t.Fatalf("live reload boundary should include provider/model fields: %+v", byApply["live"])
	}
	if !containsAnyString(byApply["restart"], "AGEZT_TELEGRAM_TOKEN") {
		t.Fatalf("restart reload boundary should include channel fields: %+v", byApply["restart"])
	}
	if !containsAnyString(byApply["restart"], "AGEZT_PEERS") || !containsAnyString(byApply["restart"], "AGEZT_TENANT_PEERS") {
		t.Fatalf("restart reload boundary should include peer mesh fields: %+v", byApply["restart"])
	}
}

func TestConfigSet_PeerMeshIsSecretRestartSetting(t *testing.T) {
	t.Setenv("AGEZT_PEERS", "")
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdConfigSet, map[string]any{
		"name":  "AGEZT_PEERS",
		"value": "edge=https://edge.example|super-secret-token",
	})
	if err != nil {
		t.Fatalf("config_set AGEZT_PEERS: %v", err)
	}
	if res["applied"] != "restart" {
		t.Fatalf("AGEZT_PEERS applied = %v, want restart; res=%+v", res["applied"], res)
	}

	values, err := c.Call(context.Background(), controlplane.CmdConfigValues, nil)
	if err != nil {
		t.Fatalf("config_values: %v", err)
	}
	fields, _ := values["fields"].([]any)
	var peer map[string]any
	for _, raw := range fields {
		row, _ := raw.(map[string]any)
		if row["env"] == "AGEZT_PEERS" {
			peer = row
			break
		}
	}
	if peer == nil {
		t.Fatalf("AGEZT_PEERS missing from config values")
	}
	if peer["secret"] != true || peer["set"] != true {
		t.Fatalf("AGEZT_PEERS should be reported as set secret: %+v", peer)
	}
	if _, ok := peer["value"]; ok {
		t.Fatalf("AGEZT_PEERS leaked a value over config_values: %+v", peer)
	}
}

func TestConfigSet_ExecutionProfilePolicyAppliesLive(t *testing.T) {
	t.Setenv("AGEZT_EXEC_PROFILE_ALLOW", "")
	t.Setenv("AGEZT_EXEC_PROFILE_DENY", "")
	t.Setenv("AGEZT_EXEC_ENV_DOCKER", "")
	t.Setenv("AGEZT_EXEC_SECRET_ENV_DOCKER", "")
	t.Setenv("AGEZT_EXEC_SECRET_FILES_DOCKER", "")
	t.Setenv("AGEZT_EXEC_SSH", "")
	t.Setenv("AGEZT_EXEC_SSH_TARGET", "")
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdConfigSet, map[string]any{
		"name":  "AGEZT_EXEC_PROFILE_DENY",
		"value": "warden",
	})
	if err != nil {
		t.Fatalf("config_set policy: %v", err)
	}
	if res["applied"] != "live" {
		t.Fatalf("policy config applied = %v, want live; res=%+v", res["applied"], res)
	}

	check, err := c.Call(context.Background(), controlplane.CmdExecutionProfileCheck, nil)
	if err != nil {
		t.Fatalf("execution_profile_check: %v", err)
	}
	profiles, _ := check["routable_run_profiles"].([]any)
	if containsAnyString(profiles, "warden") {
		t.Fatalf("policy-denied warden should not be selectable: %+v", profiles)
	}

	res, err = c.Call(context.Background(), controlplane.CmdConfigSet, map[string]any{
		"name":  "AGEZT_EXEC_PROFILE_DENY",
		"value": "",
	})
	if err != nil {
		t.Fatalf("clear policy: %v", err)
	}
	if res["applied"] != "live" {
		t.Fatalf("cleared policy applied = %v, want live; res=%+v", res["applied"], res)
	}
	check, err = c.Call(context.Background(), controlplane.CmdExecutionProfileCheck, nil)
	if err != nil {
		t.Fatalf("execution_profile_check after clear: %v", err)
	}
	profiles, _ = check["routable_run_profiles"].([]any)
	if !containsAnyString(profiles, "warden") {
		t.Fatalf("cleared policy should make warden selectable again: %+v", profiles)
	}

	for _, set := range []struct {
		name  string
		value string
	}{
		{"AGEZT_EXEC_ENV_DOCKER", "SAFE_DOCKER_ENV"},
		{"AGEZT_EXEC_SECRET_ENV_DOCKER", "OPENAI_API_KEY"},
		{"AGEZT_EXEC_SECRET_FILES_DOCKER", "OPENAI_API_KEY:openai.key"},
		{"AGEZT_EXEC_SSH", "on"},
		{"AGEZT_EXEC_SSH_TARGET", "deploy@example.com"},
	} {
		res, err = c.Call(context.Background(), controlplane.CmdConfigSet, map[string]any{
			"name":  set.name,
			"value": set.value,
		})
		if err != nil {
			t.Fatalf("config_set %s: %v", set.name, err)
		}
		if res["applied"] != "live" {
			t.Fatalf("%s applied = %v, want live; res=%+v", set.name, res["applied"], res)
		}
	}
	check, err = c.Call(context.Background(), controlplane.CmdExecutionProfileCheck, nil)
	if err != nil {
		t.Fatalf("execution_profile_check after ssh config: %v", err)
	}
	profiles, _ = check["routable_run_profiles"].([]any)
	if !containsAnyString(profiles, "ssh") {
		t.Fatalf("live SSH config should make ssh selectable: %+v", profiles)
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

func containsAnyString(values []any, want string) bool {
	for _, v := range values {
		if s, _ := v.(string); s == want {
			return true
		}
	}
	return false
}
