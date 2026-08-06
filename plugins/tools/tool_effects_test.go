// SPDX-License-Identifier: MIT

package tools_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/toolreg"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/plugins/builtintools"
	"github.com/agezt/agezt/plugins/tools/codeexec"
)

// TestFirstPartyToolDefinitionsDeclareEffects walks the REAL boot registry
// (Phase 2.2 PR 7 — this used to be a hand-listed constructor slice that
// drifted from what the daemon actually registers): builtintools.RegisterAll +
// toolreg.BuildAll under a permissive fake environment that switches every
// env-gated spec ON, then asserts every built tool's Definition().Effect is
// fully populated. A new spec added to RegisterAll is covered automatically.
//
// Two specs cannot be built hermetically through the registry and are handled
// explicitly:
//   - code_exec: its Build gates on codeexec.DetectRuntimes() finding a
//     python/node on the HOST, so a bare CI box would silently skip it — the
//     instance is constructed directly with a fixed runtime map instead (and
//     the registry-built one, when present, is covered too).
//   - plugins: the external-plugin host spawns operator-configured binaries;
//     its tools are third-party, not first-party effect-declared surface, so
//     it stays unconfigured (no AGEZT_PLUGINS) and builds nothing here.
func TestFirstPartyToolDefinitionsDeclareEffects(t *testing.T) {
	builtintools.RegisterAll()

	driver := filepath.Join(t.TempDir(), "browse.mjs")
	if err := os.WriteFile(driver, []byte("// test driver\n"), 0o600); err != nil {
		t.Fatalf("write fake driver: %v", err)
	}
	env := map[string]string{
		// switch every env-gated spec ON so the walk covers the full surface
		brand.EnvPrefix + "BROWSER_ACTIONS":         "1",
		brand.EnvPrefix + "BROWSER_ACTION_DRIVER":   driver,
		brand.EnvPrefix + "CODING_CMD":              "agent",
		brand.EnvPrefix + "ACP_AGENT_CMD":           "agent",
		brand.EnvPrefix + "HOMEASSISTANT_URL":       "https://ha.example",
		brand.EnvPrefix + "HOMEASSISTANT_TOKEN":     "tok",
		brand.EnvPrefix + "HOMEASSISTANT_TOOL_READ": "sensor.demo",
		brand.EnvPrefix + "PEERS":                   "main=https://peer.example",
	}

	var stderr bytes.Buffer
	set, err := toolreg.BuildAll(toolreg.BuildDeps{
		BaseDir:       t.TempDir(),
		WorkspaceRoot: t.TempDir(),
		Warden:        warden.New(nil),
		Stderr:        &stderr,
		Get:           func(name string) string { return env[name] },
		NotifyTargets: map[string][]string{"telegram": {"123"}}, // arms notify/send_media
	})
	if err != nil {
		t.Fatalf("BuildAll: %v; stderr=%s", err, stderr.String())
	}

	tools := set.Tools()
	// Everything the permissive env is supposed to switch on must actually be
	// in the walk — otherwise a gating change could silently shrink coverage.
	for _, want := range []string{
		"shell", "file", "http", "browser.read", "browser.action", "web_search",
		"fetch", "config", "artifacts", "db", "council", "conductor", "research",
		"schedule", "runs", "standing", "skill", "introspect", "overseer",
		"tool_forge", "mcp", "workflow", "workboard", "notify", "send_media",
		"board", "coding", "acp_agent", "homeassistant", "remote_run",
	} {
		if _, ok := tools[want]; !ok {
			t.Errorf("registry walk missing %q — its Build gate no longer passes the permissive test env", want)
		}
	}

	checked := map[string]bool{}
	checkTool := func(name string, tool agent.Tool) {
		if checked[name] {
			return
		}
		checked[name] = true
		t.Run(name, func(t *testing.T) {
			def := tool.Definition()
			if strings.TrimSpace(def.Name) == "" {
				t.Fatal("tool name is empty")
			}
			eff := def.Effect
			if eff.Class == "" {
				t.Fatal("effect class is empty")
			}
			if len(eff.PredictedEffects) == 0 {
				t.Fatal("predicted effects are empty")
			}
			if len(eff.AffectedResources) == 0 {
				t.Fatal("affected resources are empty")
			}
			if strings.TrimSpace(eff.RollbackNotes) == "" {
				t.Fatal("rollback notes are empty")
			}
			if eff.Confidence <= 0 || eff.Confidence > 1 {
				t.Fatalf("confidence=%v, want 0 < confidence <= 1", eff.Confidence)
			}
		})
	}
	for name, tool := range tools {
		if tool == nil {
			t.Fatalf("registry built a nil tool for %q", name)
		}
		checkTool(name, tool)
	}

	// code_exec: host-runtime-gated, so it may be absent from the walk above —
	// cover a directly-constructed instance with a fixed runtime map.
	ce := codeexec.NewWithWarden(warden.New(nil), t.TempDir(), map[string]string{"python": "python"}, true)
	checkTool("code_exec", ce)
}
