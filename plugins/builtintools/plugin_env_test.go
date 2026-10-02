// SPDX-License-Identifier: MIT

package builtintools

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/toolreg"
	"github.com/agezt/agezt/kernel/warden"
)

// envProbePlugin is a complete stdio plugin with one tool, "getenv", that
// reports the value of the variable named in its input as seen by the plugin
// PROCESS — the thing under test is what the daemon hands the child.
const envProbePlugin = `package main

import (
	"bufio"
	"encoding/json"
	"os"
)

type req struct {
	ID     string          ` + "`json:\"id\"`" + `
	Method string          ` + "`json:\"method\"`" + `
	Params json.RawMessage ` + "`json:\"params\"`" + `
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	for in.Scan() {
		var r req
		if json.Unmarshal(in.Bytes(), &r) != nil {
			continue
		}
		switch r.Method {
		case "initialize":
			out.Encode(map[string]any{"id": r.ID, "result": map[string]any{
				"protocol_version": 1,
				"tools": []map[string]any{{"name": "getenv", "description": "probe",
					"input_schema": map[string]any{"type": "object"}, "capability": "introspect"}},
			}})
		case "tool/invoke":
			var p struct{ Input struct{ Name string ` + "`json:\"name\"`" + ` } ` + "`json:\"input\"`" + ` }
			json.Unmarshal(r.Params, &p)
			out.Encode(map[string]any{"id": r.ID, "result": map[string]any{"output": os.Getenv(p.Input.Name)}})
		case "shutdown":
			out.Encode(map[string]any{"id": r.ID, "result": map[string]any{}})
			return
		}
	}
}
`

func buildEnvProbePlugin(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds a plugin binary")
	}
	// Not t.TempDir: buildPlugins keeps no *Plugin handle (plugins are never
	// Close()d — a known gap), so on Windows the running binary stays locked
	// and TempDir's cleanup would fail the test. The child exits on stdin EOF
	// when the test binary does; the directory is removed best-effort.
	dir, err := os.MkdirTemp("", "agezt-envprobe-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(envProbePlugin), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "envprobe")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, "main.go")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build env probe plugin: %v\n%s", err, out)
	}
	return bin
}

// TestPlugins_ChildGetsScrubbedEnvPlusGrants: a plugin child used to inherit
// the daemon's FULL environment (plugin.Config.Env was nil), so any
// third-party plugin could read every provider key. It now gets the scrubbed
// base plus exactly the variables AGEZT_PLUGIN_ENV grants it.
func TestPlugins_ChildGetsScrubbedEnvPlusGrants(t *testing.T) {
	bin := buildEnvProbePlugin(t)
	// Secret-shaped (scrubbed by name) and an ordinary daemon var (not on the
	// base allowlist): neither may reach the child unless granted.
	t.Setenv("OPENAI_API_KEY", "sk-must-not-leak")
	t.Setenv("AGEZT_VAULT_PASSPHRASE", "must-not-leak")
	t.Setenv("PROBE_GRANTED_TOKEN", "granted-ok")

	env := map[string]string{
		"AGEZT_PLUGINS":    "probe=" + bin,
		"AGEZT_PLUGIN_ENV": "probe=PROBE_GRANTED_TOKEN",
	}
	get := func(k string) string {
		if v, ok := env[k]; ok {
			return v
		}
		return os.Getenv(k)
	}
	var stderr bytes.Buffer
	built, err := buildPlugins(toolreg.BuildDeps{
		BaseDir: t.TempDir(), WorkspaceRoot: t.TempDir(), Warden: warden.New(nil), Stderr: &stderr, Get: get,
	})
	if err != nil {
		t.Fatalf("buildPlugins: %v; stderr=%s", err, stderr.String())
	}
	tool, ok := built.Extra["probe.getenv"]
	if !ok {
		t.Fatalf("plugin tool not registered; stderr=%s", stderr.String())
	}
	read := func(name string) string {
		in, _ := json.Marshal(map[string]string{"name": name})
		res, err := tool.Invoke(context.Background(), in)
		if err != nil || res.IsError {
			t.Fatalf("invoke getenv(%s): %v %+v", name, err, res)
		}
		return strings.TrimSpace(res.Output)
	}
	if got := read("OPENAI_API_KEY"); got != "" {
		t.Errorf("plugin child saw OPENAI_API_KEY=%q — the daemon's secrets leaked into a third-party process", got)
	}
	if got := read("AGEZT_VAULT_PASSPHRASE"); got != "" {
		t.Errorf("plugin child saw AGEZT_VAULT_PASSPHRASE=%q", got)
	}
	if got := read("PROBE_GRANTED_TOKEN"); got != "granted-ok" {
		t.Errorf("granted variable = %q, want granted-ok", got)
	}
	if got := read("PATH"); got == "" {
		t.Error("plugin child lost PATH — the scrubbed base must keep ordinary launch variables")
	}
}

func TestPlugins_MalformedEnvGrantIsABootError(t *testing.T) {
	_, err := buildPlugins(toolreg.BuildDeps{
		BaseDir: t.TempDir(), WorkspaceRoot: t.TempDir(), Warden: warden.New(nil), Stderr: &bytes.Buffer{},
		Get: func(k string) string {
			return map[string]string{"AGEZT_PLUGINS": "p=/bin/true", "AGEZT_PLUGIN_ENV": "no-equals-sign"}[k]
		},
	})
	if err == nil || !strings.Contains(err.Error(), "AGEZT_PLUGIN_ENV") {
		t.Fatalf("err = %v, want a hard AGEZT_PLUGIN_ENV error", err)
	}
}
