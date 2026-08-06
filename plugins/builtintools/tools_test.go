// SPDX-License-Identifier: MIT

package builtintools

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/toolreg"
	browsertool "github.com/agezt/agezt/plugins/tools/browser"
	"github.com/agezt/agezt/plugins/tools/codeexec"
	fetchtool "github.com/agezt/agezt/plugins/tools/fetch"
	httptool "github.com/agezt/agezt/plugins/tools/http"
	"github.com/agezt/agezt/plugins/tools/websearch"
)

// mapGet returns a BuildDeps.Get backed by env (missing keys → "").
func mapGet(env map[string]string) func(string) string {
	return func(name string) string { return env[name] }
}

// TestRegistryNetguardWiring_CoversEveryNetguardSpec is the registry-driven
// replacement for cmd/agezt's old hand-listed netguard type switch: it builds
// the real specs via toolreg.BuildAll, runs Set.Configure with a recording
// NetguardPublish, and asserts every Netguard spec's instance received a live
// OnBlock callback — including fetch, whose OnBlock field existed since M831
// but was never wired by wireNetguardAudit (the LD-2 live bug this PR fixes).
// NetguardGaps() being empty is the structural guard: a future Netguard spec
// whose tool forgets SetOnBlock fails here without any hand-listed case.
func TestRegistryNetguardWiring_CoversEveryNetguardSpec(t *testing.T) {
	RegisterAll()

	// Enable browser.action through the fake env (with a fake driver file), so
	// its wiring — and its verb-tool Extra family — is covered too.
	driver := filepath.Join(t.TempDir(), "browse.mjs")
	if err := os.WriteFile(driver, []byte("// test driver\n"), 0o600); err != nil {
		t.Fatalf("write fake driver: %v", err)
	}
	env := map[string]string{
		brand.EnvPrefix + "BROWSER_ACTIONS":       "1",
		brand.EnvPrefix + "BROWSER_ACTION_DRIVER": driver,
	}

	var stderr bytes.Buffer
	set, err := toolreg.BuildAll(toolreg.BuildDeps{
		BaseDir: t.TempDir(),
		Stderr:  &stderr,
		Get:     mapGet(env),
	})
	if err != nil {
		t.Fatalf("BuildAll: %v; stderr=%s", err, stderr.String())
	}

	var mu sync.Mutex
	granted := map[string]bool{} // tool names the publish factory minted callbacks for
	publish := func(tool string) func(ip, reason string) {
		mu.Lock()
		granted[tool] = true
		mu.Unlock()
		return func(ip, reason string) {}
	}
	if err := set.Configure(toolreg.KernelDeps{NetguardPublish: publish}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// The registry-driven guard: no built Netguard spec may lack NetguardAware.
	if gaps := set.NetguardGaps(); len(gaps) != 0 {
		t.Fatalf("NetguardGaps() = %v, want none — a Netguard spec's tool is missing SetOnBlock", gaps)
	}

	// Every primary Netguard instance must have been handed a callback…
	for _, want := range []string{"http", "browser.read", "browser.action", "web_search", "fetch"} {
		if !granted[want] {
			t.Errorf("NetguardPublish never invoked for %q — instance not wired", want)
		}
	}

	// …and the callback must actually be STORED on the instance (SetOnBlock is
	// not a no-op). Checked per concrete type; unlike the old cmd/agezt test
	// this is belt-and-braces on top of the registry-driven assertions above.
	tools := set.Tools()
	if ht, ok := tools["http"].(*httptool.Tool); !ok || ht.OnBlock == nil {
		t.Errorf("http OnBlock not stored (tool=%T)", tools["http"])
	}
	if br, ok := tools["browser.read"].(*browsertool.Tool); !ok || br.OnBlock == nil {
		t.Errorf("browser.read OnBlock not stored (tool=%T)", tools["browser.read"])
	}
	if ba, ok := tools["browser.action"].(*browsertool.ActionTool); !ok || ba.OnBlock == nil {
		t.Errorf("browser.action OnBlock not stored (tool=%T)", tools["browser.action"])
	}
	if ws, ok := tools["web_search"].(*websearch.Tool); !ok || ws.OnBlock == nil {
		t.Errorf("web_search OnBlock not stored (tool=%T)", tools["web_search"])
	}
	if fe, ok := tools["fetch"].(*fetchtool.Tool); !ok || fe.OnBlock == nil {
		t.Errorf("fetch OnBlock not stored (tool=%T) — the M831 LD-2 fix regressed", tools["fetch"])
	}

	// The verb tools ride along as Extra under their own names.
	for _, name := range []string{"browser.open", "browser.snapshot", "browser.click", "browser.close"} {
		if _, ok := tools[name].(*browsertool.ActionVerbTool); !ok {
			t.Errorf("%s type = %T, want *browser.ActionVerbTool", name, tools[name])
		}
	}
}

// TestRegistryAlwaysOnSpecs_BuildAndConfigure covers the Phase 2.2 PR 3+4
// specs: every always-on tool must build from an empty environment under its
// registered name with the right concrete type, and every Configure hook must
// tolerate a minimal (kernel-less) KernelDeps without error or panic — the
// tools simply stay unbound, exactly like the pre-registry boot window between
// construction and wiring. Full wired-kernel coverage stays with cmd/agezt's
// integration tests (a real *runtime.Kernel can't be cheaply built here).
func TestRegistryAlwaysOnSpecs_BuildAndConfigure(t *testing.T) {
	RegisterAll()

	var stderr bytes.Buffer
	set, err := toolreg.BuildAll(toolreg.BuildDeps{
		BaseDir: t.TempDir(),
		Stderr:  &stderr,
		Get:     mapGet(map[string]string{brand.EnvPrefix + "SANDBOX": "off"}),
	})
	if err != nil {
		t.Fatalf("BuildAll: %v; stderr=%s", err, stderr.String())
	}
	tools := set.Tools()

	// The Set*-injection batch + the kernel-bound zero-arg tools are always on.
	alwaysOn := []string{
		"config", "artifacts", "db", "council", "conductor", "research",
		"schedule", "runs", "standing", "skill", "introspect", "overseer",
		"tool_forge", "mcp", "workflow", "workboard",
	}
	for _, name := range alwaysOn {
		tl, ok := tools[name]
		if !ok {
			t.Errorf("always-on tool %q not built", name)
			continue
		}
		if got := tl.Definition().Name; got != name {
			t.Errorf("tool %q Definition().Name = %q — registry key and definition drifted", name, got)
		}
	}

	// AGEZT_SANDBOX=off gates code_exec off regardless of host runtimes.
	if _, ok := tools["code_exec"]; ok {
		t.Error("code_exec built while AGEZT_SANDBOX=off")
	}

	// Configure with minimal deps (no kernel, no artifact index, no lake, no
	// bus) must be a safe no-op for every hook: nothing to bind, no error.
	if err := set.Configure(toolreg.KernelDeps{}); err != nil {
		t.Fatalf("Configure with minimal deps: %v", err)
	}

	// PreOpen with a zero runtime.Config must also be safe — code_exec was
	// gated off, so no spec should touch cfg.ScriptRunner here.
	var cfg runtime.Config
	set.ApplyPreOpen(&cfg)
	if cfg.ScriptRunner != nil {
		t.Error("ApplyPreOpen set ScriptRunner while code_exec is gated off")
	}
}

// TestRegistryCodeExecGating pins code_exec's env gate: off under
// AGEZT_SANDBOX=off, and when host runtimes exist it builds, contributes its
// banner desc, and PreOpen wires cfg.ScriptRunner to the built instance.
func TestRegistryCodeExecGating(t *testing.T) {
	RegisterAll()

	var stderr bytes.Buffer
	set, err := toolreg.BuildAll(toolreg.BuildDeps{
		BaseDir: t.TempDir(),
		Stderr:  &stderr,
		Get:     mapGet(nil),
	})
	if err != nil {
		t.Fatalf("BuildAll: %v; stderr=%s", err, stderr.String())
	}
	ce, ok := set.Tools()["code_exec"].(*codeexec.Tool)
	if !ok {
		t.Skip("no code_exec runtimes on this host — gating covered by TestRegistryAlwaysOnSpecs_BuildAndConfigure")
	}
	if len(ce.Languages()) == 0 {
		t.Error("built code_exec reports no languages")
	}
	found := false
	for _, d := range set.Descs() {
		if strings.HasPrefix(d, "code_exec(langs=") {
			found = true
		}
	}
	if !found {
		t.Errorf("Descs() missing code_exec banner: %v", set.Descs())
	}
	var cfg runtime.Config
	set.ApplyPreOpen(&cfg)
	if got, ok := cfg.ScriptRunner.(*codeexec.Tool); !ok || got != ce {
		t.Errorf("ApplyPreOpen wired cfg.ScriptRunner = %T, want the built code_exec instance", cfg.ScriptRunner)
	}
}

// TestRegistryBrowserActionGatedOff pins the opt-in: without
// AGEZT_BROWSER_ACTIONS=1 the browser.action spec builds nothing, contributes
// no verb tools, and — having built nothing — is not a netguard gap.
func TestRegistryBrowserActionGatedOff(t *testing.T) {
	RegisterAll()

	var stderr bytes.Buffer
	set, err := toolreg.BuildAll(toolreg.BuildDeps{
		BaseDir: t.TempDir(),
		Stderr:  &stderr,
		Get:     mapGet(nil),
	})
	if err != nil {
		t.Fatalf("BuildAll: %v; stderr=%s", err, stderr.String())
	}
	tools := set.Tools()
	if _, ok := tools["browser.action"]; ok {
		t.Fatal("browser.action built while AGEZT_BROWSER_ACTIONS is unset")
	}
	for name := range tools {
		if strings.HasPrefix(name, "browser.") && name != "browser.read" {
			t.Errorf("unexpected verb tool %q while browser.action is gated off", name)
		}
	}
	for _, want := range []string{"fetch", "http", "web_search"} {
		if _, ok := tools[want]; !ok {
			t.Errorf("always-on network tool %q missing from Tools()", want)
		}
	}
	if gaps := set.NetguardGaps(); len(gaps) != 0 {
		t.Errorf("NetguardGaps() = %v — a spec that built nothing must not be a gap", gaps)
	}
}
