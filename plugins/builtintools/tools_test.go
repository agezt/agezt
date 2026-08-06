// SPDX-License-Identifier: MIT

package builtintools

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/toolreg"
	browsertool "github.com/agezt/agezt/plugins/tools/browser"
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
	got := make([]string, 0, len(tools))
	for name := range tools {
		got = append(got, name)
	}
	sort.Strings(got)
	if want := "fetch,http,web_search"; !strings.Contains(strings.Join(got, ","), want) {
		t.Errorf("Tools() = %v, want the always-on trio %s present", got, want)
	}
	if gaps := set.NetguardGaps(); len(gaps) != 0 {
		t.Errorf("NetguardGaps() = %v — a spec that built nothing must not be a gap", gaps)
	}
}
