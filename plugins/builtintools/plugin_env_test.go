// SPDX-License-Identifier: MIT

package builtintools

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/pluginapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/toolreg"
	"github.com/agezt/agezt/kernel/warden"
)

// fakeSpawner is the contract's spawn port, answered by a fake: it records
// what the plugin asked for and hands back a fake plugin.
//
// The plugin side of this package may not name the host's module, so the
// plugin is built against the PORT — and the port is what the test answers.
// The other half of the promise (that the host really hands the child the
// environment the plugin asked for) is measured in the app layer, where the
// real spawn lives.
type fakeSpawner struct {
	reqs   []pluginapi.SpawnRequest
	closed bool
}

func (s *fakeSpawner) Spawn(_ context.Context, req pluginapi.SpawnRequest) (pluginapi.Spawned, error) {
	s.reqs = append(s.reqs, req)
	return &fakeSpawned{s: s}, nil
}

// fakeSpawned is a spawned plugin that answers one tool.
type fakeSpawned struct{ s *fakeSpawner }

// probeTool is the fake plugin's one tool.
type probeTool struct {
	name   string
	closed *bool
}

func (t probeTool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{Name: t.name, Description: "probe"}
}

func (t probeTool) Invoke(_ context.Context, _ json.RawMessage) (toolapi.Result, error) {
	if t.closed != nil && *t.closed {
		return toolapi.Result{Output: "plugin closed", IsError: true}, nil
	}
	return toolapi.Result{Output: "ok"}, nil
}

func (f *fakeSpawned) Tools(prefix string) map[string]toolapi.Tool {
	return map[string]toolapi.Tool{
		prefix + "getenv": probeTool{name: prefix + "getenv", closed: &f.s.closed},
	}
}

func (f *fakeSpawned) ToolCapabilities(prefix string) map[string]string { return nil }

func (f *fakeSpawned) Close() error {
	f.s.closed = true
	return nil
}

// TestPlugins_ChildGetsScrubbedEnvPlusGrants: a plugin child used to inherit
// the daemon's FULL environment (the spawn configuration's Env was nil), so
// any third-party plugin could read every provider key. It now gets the
// scrubbed base plus exactly the variables AGEZT_PLUGIN_ENV grants it.
//
// The plugin side of that promise is what this test measures: the
// ENVIRONMENT the plugin asks for. That the host hands the child exactly
// this environment is the host side's promise, measured in the app layer
// (kernel/app/pluginhost) against a real child process.
func TestPlugins_ChildGetsScrubbedEnvPlusGrants(t *testing.T) {
	// Secret-shaped (scrubbed by name) and an ordinary daemon var (not on the
	// base allowlist): neither may reach the child unless granted.
	t.Setenv("OPENAI_API_KEY", "sk-must-not-leak")
	t.Setenv("AGEZT_VAULT_PASSPHRASE", "must-not-leak")
	t.Setenv("PROBE_GRANTED_TOKEN", "granted-ok")

	env := map[string]string{
		"AGEZT_PLUGINS":    "probe=/bin/probe",
		"AGEZT_PLUGIN_ENV": "probe=PROBE_GRANTED_TOKEN",
	}
	// The operator's binary pin and tool allowlist must reach the spawn
	// request: the host enforces them, and the plugin is the one that knows
	// which prefix they belong to.
	pin := strings.Repeat("a", 64)
	env["AGEZT_PLUGIN_PINS"] = "probe=" + pin
	env["AGEZT_PLUGIN_TOOLS"] = "probe=getenv"
	get := func(k string) string {
		if v, ok := env[k]; ok {
			return v
		}
		return os.Getenv(k)
	}
	var stderr bytes.Buffer
	spawner := &fakeSpawner{}
	built, err := buildPlugins(toolreg.BuildDeps{
		BaseDir: t.TempDir(), WorkspaceRoot: t.TempDir(), Warden: warden.New(nil), Stderr: &stderr, Get: get,
		Plugins: spawner,
	})
	if err != nil {
		t.Fatalf("buildPlugins: %v; stderr=%s", err, stderr.String())
	}
	if len(spawner.reqs) != 1 {
		t.Fatalf("the plugin asked to spawn %d plugins, want 1", len(spawner.reqs))
	}
	req := spawner.reqs[0]
	if req.PinnedHash != pin {
		t.Errorf("the spawn request carries pin %q, want the operator's %q", req.PinnedHash, pin)
	}
	if len(req.AllowedTools) != 1 || req.AllowedTools[0] != "getenv" {
		t.Errorf("the spawn request carries allowlist %v, want [getenv]", req.AllowedTools)
	}
	childEnv := req.Env
	has := func(name string) (string, bool) {
		for _, kv := range childEnv {
			if v, ok := strings.CutPrefix(kv, name+"="); ok {
				return v, true
			}
		}
		return "", false
	}
	if v, ok := has("OPENAI_API_KEY"); ok {
		t.Errorf("the plugin asked to hand OPENAI_API_KEY=%q to the child — the daemon's secrets would leak into a third-party process", v)
	}
	if v, ok := has("AGEZT_VAULT_PASSPHRASE"); ok {
		t.Errorf("the plugin asked to hand AGEZT_VAULT_PASSPHRASE=%q to the child", v)
	}
	if v, ok := has("PROBE_GRANTED_TOKEN"); !ok || v != "granted-ok" {
		t.Errorf("granted variable = %q (present=%v), want granted-ok", v, ok)
	}
	if v, ok := has("PATH"); !ok || v == "" {
		t.Error("the plugin asked to hand the child no PATH — the scrubbed base must keep ordinary launch variables")
	}

	// The plugin's tools register under the prefix.
	if _, ok := built.Extra["probe.getenv"]; !ok {
		t.Fatalf("plugin tool not registered; stderr=%s", stderr.String())
	}

	// The daemon closes plugin children at shutdown through Built.Close; they
	// used to be left running.
	if built.Close == nil {
		t.Fatal("buildPlugins spawned a plugin but returned no Close")
	}
	if err := built.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !spawner.closed {
		t.Fatal("Close did not close the spawned plugin")
	}
	tool := built.Extra["probe.getenv"]
	in, _ := json.Marshal(map[string]string{"name": "PATH"})
	if res, err := tool.Invoke(context.Background(), in); err == nil && !res.IsError {
		t.Fatal("plugin still answering after Close")
	}
}

// TestPlugins_WithoutTheSpawnPortIsABootError: the plugin is built against
// the contract's port, and a daemon that handed over no port leaves the
// plugin unable to spawn anything — it must say so rather than call into
// nothing.
func TestPlugins_WithoutTheSpawnPortIsABootError(t *testing.T) {
	var stderr bytes.Buffer
	_, err := buildPlugins(toolreg.BuildDeps{
		BaseDir: t.TempDir(), WorkspaceRoot: t.TempDir(), Warden: warden.New(nil), Stderr: &stderr,
		Get: func(k string) string {
			if k == "AGEZT_PLUGINS" {
				return "probe=/bin/probe"
			}
			return ""
		},
	})
	if err == nil || !strings.Contains(err.Error(), "plugin host is not available") {
		t.Fatalf("err = %v, want a hard 'the plugin host is not available' error", err)
	}
}

func TestPlugins_MalformedEnvGrantIsABootError(t *testing.T) {
	_, err := buildPlugins(toolreg.BuildDeps{
		BaseDir: t.TempDir(), WorkspaceRoot: t.TempDir(), Warden: warden.New(nil), Stderr: &bytes.Buffer{},
		Plugins: &fakeSpawner{},
		Get: func(k string) string {
			return map[string]string{"AGEZT_PLUGINS": "p=/bin/true", "AGEZT_PLUGIN_ENV": "no-equals-sign"}[k]
		},
	})
	if err == nil || !strings.Contains(err.Error(), "AGEZT_PLUGIN_ENV") {
		t.Fatalf("err = %v, want a hard AGEZT_PLUGIN_ENV error", err)
	}
}
