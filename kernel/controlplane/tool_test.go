// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"

	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

// TestToolList_ReturnsRegisteredTools verifies the wire shape:
// CmdToolList returns a `tools` array sorted by name, each entry
// carrying {name, description}. The default startPair rig wires
// one tool ("shell"), so this asserts the minimal happy path.
func TestToolList_ReturnsRegisteredTools(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdToolList, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	if got := intOf(res["count"]); got != 1 {
		t.Errorf("count = %d want 1", got)
	}
	rows, ok := res["tools"].([]any)
	if !ok {
		t.Fatalf("tools wrong type: %T", res["tools"])
	}
	if len(rows) != 1 {
		t.Fatalf("tools len = %d want 1", len(rows))
	}
	row, _ := rows[0].(map[string]any)
	if row["name"] != "shell" {
		t.Errorf("name = %v want shell", row["name"])
	}
	// Description comes from shell.NewWithWarden(warden.New(nil)).Definition(); we don't pin
	// the exact text (it can evolve) but it must be non-empty so the
	// CLI has something to render.
	if desc, _ := row["description"].(string); desc == "" {
		t.Error("description is empty; CLI would render a blank column")
	}
	if row["effect_class"] != "irreversible" || row["rollback_mode"] != "audit_only" {
		t.Fatalf("rollback metadata = class %v mode %v, want irreversible/audit_only", row["effect_class"], row["rollback_mode"])
	}
	if notes, _ := row["rollback_notes"].(string); !strings.Contains(notes, "No reliable generic rollback") {
		t.Fatalf("rollback notes = %q", notes)
	}
}

// TestToolList_EmptyWhenNoToolsRegistered covers the degenerate
// case — a kernel constructed with no tools (rare but legal, e.g.
// a planner-only daemon). The response must still be a valid
// JSON array, not null, so downstream jq pipelines don't break.
func TestToolList_EmptyWhenNoToolsRegistered(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{
		BaseDir:  dir,
		Provider: mock.New(mock.FinalText("ok")),
		// Tools omitted — kernel runs with no in-process tools.
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	srv := controlplane.NewServer(k, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	client, err := dialUntilReady(t, dir)
	if err != nil {
		t.Fatal(err)
	}

	res, err := client.Call(context.Background(), controlplane.CmdToolList, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["count"]); got != 0 {
		t.Errorf("count = %d want 0", got)
	}
	rows, ok := res["tools"].([]any)
	if !ok {
		t.Fatalf("tools wrong type: %T (want []any even when empty)", res["tools"])
	}
	if len(rows) != 0 {
		t.Errorf("tools should be empty, got %d rows", len(rows))
	}
}

// TestToolList_SortsByName ensures the deterministic-output
// promise the handler doc makes — operators piping into diff,
// jq, or just visually scanning rely on a stable order across
// calls. Wires two tools so map-iteration randomness has a
// chance to surface in non-sorted output.
func TestToolList_SortsByName(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{
		BaseDir:  dir,
		Provider: mock.New(mock.FinalText("ok")),
		Tools: map[string]agent.Tool{
			"zeta":  shell.NewWithWarden(warden.New(nil)),
			"alpha": shell.NewWithWarden(warden.New(nil)),
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	srv := controlplane.NewServer(k, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	client, err := dialUntilReady(t, dir)
	if err != nil {
		t.Fatal(err)
	}

	// Both fixture tools wrap shell.NewWithWarden(warden.New(nil)), so they advertise the
	// SAME Definition().Name ("shell"). The handler sorts on the
	// definition name, not the map key — verify the count is 2
	// and the order is stable across repeated calls.
	for i := 0; i < 3; i++ {
		res, err := client.Call(context.Background(), controlplane.CmdToolList, nil)
		if err != nil {
			t.Fatalf("Call %d: %v", i, err)
		}
		rows, _ := res["tools"].([]any)
		if len(rows) != 2 {
			t.Fatalf("call %d: tools len = %d want 2", i, len(rows))
		}
		a, _ := rows[0].(map[string]any)
		b, _ := rows[1].(map[string]any)
		na, _ := a["name"].(string)
		nb, _ := b["name"].(string)
		if na > nb {
			t.Errorf("call %d: not sorted: %q > %q", i, na, nb)
		}
	}
}

func TestAgentPermissions_ReturnsEffectiveToolPolicy(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{
		BaseDir:  dir,
		Provider: mock.New(mock.FinalText("ok")),
		Tools: map[string]agent.Tool{
			"alpha": testTool{name: "alpha"},
			"beta":  testTool{name: "beta"},
			"gamma": testTool{name: "gamma"},
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	if _, err := k.AddProfile(roster.Profile{
		Slug:         "guarded",
		ToolAllow:    []string{"alpha"},
		ToolDeny:     []string{"beta"},
		TrustCeiling: "L2",
	}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	directFalse := false
	if _, err := k.AddProfile(roster.Profile{
		Slug:           "worker",
		ParentAgent:    "guarded",
		DirectCallable: &directFalse,
	}); err != nil {
		t.Fatalf("AddProfile worker: %v", err)
	}
	k.Edict().SetLevel("alpha", edict.LevelAllow)
	k.Edict().SetLevel("beta", edict.LevelAllow)
	k.Edict().SetLevel("gamma", edict.LevelAllow)
	global := configcenter.NewConfigEntry("public:value", "public")
	guardedOnly := configcenter.NewConfigEntry("agent/guarded/runtime", "mode=careful")
	guardedOnly.AllowedAgents = []string{"guarded"}
	guardedOnly.Metadata = map[string]string{"agent": "guarded"}
	otherOnly := configcenter.NewConfigEntry("agent/other/runtime", "mode=other")
	otherOnly.AllowedAgents = []string{"other"}
	excluded := configcenter.NewConfigEntry("blocked:value", "blocked")
	excluded.ExcludedAgents = []string{"guarded"}
	for _, entry := range []*configcenter.ConfigEntry{global, guardedOnly, otherOnly, excluded} {
		if err := k.ConfigCenter().Set(entry); err != nil {
			t.Fatalf("config set %s: %v", entry.Key, err)
		}
	}

	srv := controlplane.NewServer(k, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })
	client, err := dialUntilReady(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.Call(context.Background(), controlplane.CmdAgentPermissions, map[string]any{"ref": "guarded"})
	if err != nil {
		t.Fatalf("agent permissions: %v", err)
	}
	if res["slug"] != "guarded" || res["trust_ceiling"] != "L2" {
		t.Fatalf("permission header = %+v", res)
	}
	wake, _ := res["wake_access"].(map[string]any)
	if wake["direct_allowed"] != true || wake["schedule_allowed"] != true || wake["channel_allowed"] != true || wake["delegation_scope"] != "any" {
		t.Fatalf("direct agent wake access wrong: %+v", wake)
	}
	rows, _ := res["permissions"].([]any)
	byName := map[string]map[string]any{}
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		name, _ := row["name"].(string)
		byName[name] = row
	}
	if byName["alpha"]["status"] != "L2" || byName["alpha"]["source"] != "edict" || byName["alpha"]["ask"] != true {
		t.Fatalf("alpha should be allowed via Edict clamped to L2: %+v", byName["alpha"])
	}
	if byName["beta"]["status"] != "denied" || byName["beta"]["source"] != "agent_deny" {
		t.Fatalf("beta should be agent-denied: %+v", byName["beta"])
	}
	if byName["gamma"]["status"] != "hidden" || byName["gamma"]["source"] != "agent_allow" {
		t.Fatalf("gamma should be hidden by allowlist: %+v", byName["gamma"])
	}
	configRows, _ := res["config_entries"].([]any)
	byKey := map[string]map[string]any{}
	for _, raw := range configRows {
		row, _ := raw.(map[string]any)
		key, _ := row["key"].(string)
		byKey[key] = row
	}
	if byKey["public:value"]["visible"] != true || byKey["public:value"]["source"] != "config_global" {
		t.Fatalf("public config should be globally visible: %+v", byKey["public:value"])
	}
	if byKey["agent/guarded/runtime"]["visible"] != true || byKey["agent/guarded/runtime"]["source"] != "config_allowed" || byKey["agent/guarded/runtime"]["owned"] != true {
		t.Fatalf("guarded config should be visible and owned: %+v", byKey["agent/guarded/runtime"])
	}
	if byKey["agent/other/runtime"]["visible"] != false || byKey["agent/other/runtime"]["source"] != "config_allowed" {
		t.Fatalf("other-only config should be hidden by allowed_agents: %+v", byKey["agent/other/runtime"])
	}
	if byKey["blocked:value"]["visible"] != false || byKey["blocked:value"]["source"] != "config_excluded" {
		t.Fatalf("excluded config should be hidden by excluded_agents: %+v", byKey["blocked:value"])
	}
	governance, _ := res["governance"].(map[string]any)
	if intOf(governance["tool_count"]) != 3 || intOf(governance["allowed_count"]) != 0 || intOf(governance["ask_count"]) != 1 || intOf(governance["blocked_count"]) != 2 {
		t.Fatalf("governance tool counts wrong: %+v", governance)
	}
	if intOf(governance["tool_allow_count"]) != 1 || intOf(governance["tool_deny_count"]) != 1 || governance["trust_ceiling"] != "L2" {
		t.Fatalf("governance tool policy wrong: %+v", governance)
	}
	if got := strings.Join(anyStrings(governance["direct_tools"]), ","); got != "" {
		t.Fatalf("direct_tools = %q, want none because alpha is ask-gated by L2", got)
	}
	if got := strings.Join(anyStrings(governance["ask_tools"]), ","); got != "alpha" {
		t.Fatalf("ask_tools = %q, want alpha", got)
	}
	if got := strings.Join(anyStrings(governance["blocked_tools"]), ","); got != "beta,gamma" {
		t.Fatalf("blocked_tools = %q, want beta,gamma", got)
	}
	if intOf(governance["config_count"]) != 4 || intOf(governance["config_visible_count"]) != 2 || intOf(governance["config_owned_count"]) != 1 || intOf(governance["config_hidden_count"]) != 2 {
		t.Fatalf("governance config counts wrong: %+v", governance)
	}
	if got := strings.Join(anyStrings(governance["visible_configs"]), ","); got != "agent/guarded/runtime,public:value" {
		t.Fatalf("visible_configs = %q, want agent/guarded/runtime,public:value", got)
	}
	if got := strings.Join(anyStrings(governance["hidden_configs"]), ","); got != "agent/other/runtime,blocked:value" {
		t.Fatalf("hidden_configs = %q, want agent/other/runtime,blocked:value", got)
	}
	if governance["risk"] != "restricted" || governance["summary"] == "" {
		t.Fatalf("governance summary wrong: %+v", governance)
	}
	if governance["tool_policy"] != "allowlist+denylist" || governance["memory_policy"] != "default:guarded" || governance["memory_writes"] != "enabled" {
		t.Fatalf("governance authority policy wrong: %+v", governance)
	}
	if got, _ := governance["authority_boundary"].(string); !strings.Contains(got, "direct agent") {
		t.Fatalf("authority_boundary = %q, want direct agent", got)
	}
	if got, _ := governance["execution_boundary"].(string); !strings.Contains(got, "schedules/workflows invoke through this policy") {
		t.Fatalf("execution_boundary = %q, want schedule/workflow boundary", got)
	}
	if got, _ := governance["permission_passport"].(string); !strings.Contains(got, "trust L2") || !strings.Contains(got, "tools allowlist+denylist") || !strings.Contains(got, "memory default:guarded") {
		t.Fatalf("permission_passport = %q, want trust/tool/memory passport", got)
	}

	workerRes, err := client.Call(context.Background(), controlplane.CmdAgentPermissions, map[string]any{"ref": "worker"})
	if err != nil {
		t.Fatalf("worker permissions: %v", err)
	}
	workerWake, _ := workerRes["wake_access"].(map[string]any)
	if workerWake["direct_allowed"] != false || workerWake["schedule_allowed"] != false || workerWake["channel_allowed"] != false {
		t.Fatalf("managed sub-agent direct wake access should be blocked: %+v", workerWake)
	}
	if workerWake["delegation_allowed"] != true || workerWake["delegation_scope"] != "manager" || workerWake["manager"] != "guarded" {
		t.Fatalf("managed sub-agent delegation access wrong: %+v", workerWake)
	}
	sources, _ := workerWake["delegation_sources"].([]any)
	if len(sources) != 1 || sources[0] != "guarded" {
		t.Fatalf("managed sub-agent delegation sources wrong: %+v", sources)
	}
	workerGovernance, _ := workerRes["governance"].(map[string]any)
	if got, _ := workerGovernance["authority_boundary"].(string); !strings.Contains(got, "managed sub-agent") {
		t.Fatalf("worker authority_boundary = %q, want managed sub-agent", got)
	}
}

func TestAgentCapabilities_PatchesResourceAuthority(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{
		BaseDir:  dir,
		Provider: mock.New(mock.FinalText("ok")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	if _, err := k.AddProfile(roster.Profile{Slug: "builder"}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	srv := controlplane.NewServer(k, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })
	client, err := dialUntilReady(t, dir)
	if err != nil {
		t.Fatal(err)
	}

	res, err := client.Call(context.Background(), controlplane.CmdAgentCapabilities, map[string]any{
		"ref":          "builder",
		"memory_scope": "agent/builder",
		"workdir":      "agents/builder",
		"max_cost_mc":  float64(100),
		"max_daily_mc": float64(500),
	})
	if err != nil {
		t.Fatalf("agent capabilities resource patch: %v", err)
	}
	prof, _ := res["profile"].(map[string]any)
	if prof["memory_scope"] != "agent/builder" || prof["workdir"] != "agents/builder" ||
		intOf(prof["max_cost_mc"]) != 100 || intOf(prof["max_daily_mc"]) != 500 {
		t.Fatalf("resource authority fields not patched: %+v", prof)
	}
	if _, err := client.Call(context.Background(), controlplane.CmdAgentCapabilities, map[string]any{
		"ref":     "builder",
		"workdir": "../escape",
	}); err == nil || !strings.Contains(err.Error(), "workdir must be a relative path") {
		t.Fatalf("unsafe workdir err = %v, want roster validation", err)
	}
}

type testTool struct{ name string }

func (t testTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: t.name, Description: "test tool", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (t testTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	return agent.Result{Output: "ok"}, nil
}
