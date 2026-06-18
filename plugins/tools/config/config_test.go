// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func invoke(t *testing.T, tool *Tool, args map[string]any) (string, bool) {
	t.Helper()
	raw, _ := json.Marshal(args)
	res, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("invoke %v: %v", args, err)
	}
	return res.Output, res.IsError
}

func TestConfigTool_RegisterSchemaSetGet(t *testing.T) {
	t.Setenv("AGEZT_VAULT_PASSPHRASE", "test-pass")
	dir := t.TempDir()
	tool := New(dir)

	// register a namespaced section with a secret + a plain field
	section := map[string]any{
		"id":   "weather",
		"name": "Weather Skill",
		"fields": []map[string]any{
			{"env": "AGEZT_X_WEATHER_API_KEY", "label": "API key", "type": "password", "secret": true},
			{"env": "AGEZT_X_WEATHER_UNITS", "label": "Units", "type": "text"},
		},
	}
	if out, isErr := invoke(t, tool, map[string]any{"op": "register", "section": section}); isErr {
		t.Fatalf("register failed: %s", out)
	}

	// schema lists it as registered
	out, _ := invoke(t, tool, map[string]any{"op": "schema"})
	if !strings.Contains(out, "weather") || !strings.Contains(out, "(registered)") {
		t.Fatalf("schema missing registered section:\n%s", out)
	}

	// set the plain field → restart-class
	out, isErr := invoke(t, tool, map[string]any{"op": "set", "name": "AGEZT_X_WEATHER_UNITS", "value": "metric"})
	if isErr || !strings.Contains(out, "restart to apply") {
		t.Fatalf("set units: out=%q isErr=%v", out, isErr)
	}
	if out, _ := invoke(t, tool, map[string]any{"op": "get", "name": "AGEZT_X_WEATHER_UNITS"}); out != "AGEZT_X_WEATHER_UNITS=metric" {
		t.Fatalf("get units: %q", out)
	}

	// set the secret → vault; get reports presence only (never the value)
	if _, isErr := invoke(t, tool, map[string]any{"op": "set", "name": "AGEZT_X_WEATHER_API_KEY", "value": "FAKE-KEY"}); isErr {
		t.Fatal("set secret failed")
	}
	out, _ = invoke(t, tool, map[string]any{"op": "get", "name": "AGEZT_X_WEATHER_API_KEY"})
	if strings.Contains(out, "FAKE-KEY") {
		t.Fatalf("secret value leaked via get: %q", out)
	}
	if !strings.Contains(out, "set (secret") {
		t.Fatalf("secret presence not reported: %q", out)
	}

	// unregister
	if out, isErr := invoke(t, tool, map[string]any{"op": "unregister", "id": "weather"}); isErr || !strings.Contains(out, "unregistered") {
		t.Fatalf("unregister: out=%q isErr=%v", out, isErr)
	}
}

func TestConfigTool_RejectsBadOps(t *testing.T) {
	tool := New(t.TempDir())
	cases := []map[string]any{
		{"op": "wat"},
		{"op": "get", "name": "AGEZT_NOPE"},               // unknown setting
		{"op": "set", "name": "AGEZT_NOPE", "value": "x"}, // unknown setting
		{"op": "register", "section": map[string]any{"id": "evil", "name": "e", "fields": []map[string]any{{"env": "AGEZT_ALLOW_ALL", "type": "bool"}}}}, // shadows built-in
		{"op": "register", "section": map[string]any{"id": "ns", "name": "n", "fields": []map[string]any{{"env": "OPENAI_KEY", "type": "text"}}}},        // not namespaced
	}
	for _, c := range cases {
		out, isErr := invoke(t, tool, c)
		if !isErr {
			t.Errorf("expected error for %v, got %q", c, out)
		}
	}
}

func TestConfigTool_ReadOnlyAndLocked(t *testing.T) {
	dir := t.TempDir()
	tool := New(dir)
	section := map[string]any{
		"id":   "sysmanaged",
		"name": "System Managed",
		"fields": []map[string]any{
			{"env": "AGEZT_X_SYS_RO", "label": "Read only", "type": "text", "read_only": true},
			{"env": "AGEZT_X_SYS_LOCKED", "label": "Locked", "type": "text", "locked": true},
		},
	}
	if _, isErr := invoke(t, tool, map[string]any{"op": "register", "section": section}); isErr {
		t.Fatal("register failed")
	}
	// read-only: any set is refused.
	if out, isErr := invoke(t, tool, map[string]any{"op": "set", "name": "AGEZT_X_SYS_RO", "value": "x"}); !isErr || !strings.Contains(out, "read-only") {
		t.Errorf("read-only set should fail: out=%q isErr=%v", out, isErr)
	}
	// locked: updating to a value is fine...
	if _, isErr := invoke(t, tool, map[string]any{"op": "set", "name": "AGEZT_X_SYS_LOCKED", "value": "v1"}); isErr {
		t.Error("locked set to a value should succeed")
	}
	// ...but clearing it is refused.
	if out, isErr := invoke(t, tool, map[string]any{"op": "set", "name": "AGEZT_X_SYS_LOCKED", "value": ""}); !isErr || !strings.Contains(out, "locked") {
		t.Errorf("locked clear should fail: out=%q isErr=%v", out, isErr)
	}
}

func TestConfigTool_LockedSectionForce(t *testing.T) {
	dir := t.TempDir()
	tool := New(dir)
	section := map[string]any{
		"id":     "sys-section",
		"name":   "Sys Section",
		"locked": true,
		"fields": []map[string]any{{"env": "AGEZT_X_S_A", "label": "A", "type": "text"}},
	}
	if _, isErr := invoke(t, tool, map[string]any{"op": "register", "section": section}); isErr {
		t.Fatal("register failed")
	}
	if out, isErr := invoke(t, tool, map[string]any{"op": "unregister", "id": "sys-section"}); !isErr || !strings.Contains(out, "locked") {
		t.Errorf("locked section unregister should fail without force: out=%q isErr=%v", out, isErr)
	}
	if out, isErr := invoke(t, tool, map[string]any{"op": "unregister", "id": "sys-section", "force": true}); isErr || !strings.Contains(out, "unregistered") {
		t.Errorf("force unregister should succeed: out=%q isErr=%v", out, isErr)
	}
}

func TestConfigTool_GetSetBuiltin(t *testing.T) {
	t.Setenv("AGEZT_MODEL", "") // ignore any ambient env so get reads the store
	dir := t.TempDir()
	tool := New(dir)
	// A built-in non-secret field round-trips through the store (no kernel bound,
	// so even a live field reports restart rather than reloading).
	if out, isErr := invoke(t, tool, map[string]any{"op": "set", "name": "AGEZT_MODEL", "value": "deepseek-chat"}); isErr {
		t.Fatalf("set builtin model: %s", out)
	}
	if out, _ := invoke(t, tool, map[string]any{"op": "get", "name": "AGEZT_MODEL"}); out != "AGEZT_MODEL=deepseek-chat" {
		t.Fatalf("get builtin model: %q", out)
	}
}

func TestConfigTool_AgentScopedOverrides(t *testing.T) {
	dir := t.TempDir()
	tool := New(dir)
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  dir,
		Provider: mock.New(mock.FinalText("ok")),
	})
	if err != nil {
		t.Fatalf("runtime open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	tool.SetKernel(k)

	if _, err := k.AddProfile(roster.Profile{
		Slug: "researcher",
		ConfigOverrides: map[string]string{
			"AGEZT_MODEL": "agent-model",
		},
	}); err != nil {
		t.Fatalf("add profile: %v", err)
	}

	ctx := kernelruntime.WithAgentProfile(context.Background(), roster.Profile{
		Slug: "researcher",
		ConfigOverrides: map[string]string{
			"AGEZT_MODEL": "agent-model",
		},
	})

	raw, _ := json.Marshal(map[string]any{"op": "get", "name": "AGEZT_MODEL"})
	res, err := tool.Invoke(ctx, raw)
	if err != nil {
		t.Fatalf("get effective: %v", err)
	}
	if res.Output != "AGEZT_MODEL=agent-model (agent override: researcher)" {
		t.Fatalf("effective get = %q", res.Output)
	}

	raw, _ = json.Marshal(map[string]any{"op": "set", "scope": "agent", "name": "AGEZT_MODEL", "value": "agent-next"})
	res, err = tool.Invoke(ctx, raw)
	if err != nil || res.IsError {
		t.Fatalf("set agent override: out=%q err=%v isErr=%v", res.Output, err, res.IsError)
	}
	got, ok := k.Roster().Get("researcher")
	if !ok || got.ConfigOverrides["AGEZT_MODEL"] != "agent-next" {
		t.Fatalf("profile override not saved: %+v ok=%v", got.ConfigOverrides, ok)
	}

	raw, _ = json.Marshal(map[string]any{"op": "set", "scope": "agent", "name": "AGEZT_MODEL", "value": ""})
	res, err = tool.Invoke(ctx, raw)
	if err != nil || res.IsError {
		t.Fatalf("clear agent override: out=%q err=%v isErr=%v", res.Output, err, res.IsError)
	}
	got, _ = k.Roster().Get("researcher")
	if _, ok := got.ConfigOverrides["AGEZT_MODEL"]; ok {
		t.Fatalf("profile override should be cleared: %+v", got.ConfigOverrides)
	}
}
