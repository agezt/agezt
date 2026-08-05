// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/warden"
	browsertool "github.com/agezt/agezt/plugins/tools/browser"
	httptool "github.com/agezt/agezt/plugins/tools/http"
	"github.com/agezt/agezt/plugins/tools/websearch"
)

// TestWireNetguardAudit_CoversAllNetguardCapableTools guards against the key
// drift that silently unwired browser netguard auditing: main.go asserted
// tools["browser"] while boot_tools.go registered "browser.read", so the
// OnBlock callback was never installed and SSRF refusals went unjournaled.
// The test builds the REAL tool map via buildTools (so registration keys are
// the production ones), wires the audit, then walks every map entry: any tool
// whose concrete type carries an OnBlock hook must have received it.
func TestWireNetguardAudit_CoversAllNetguardCapableTools(t *testing.T) {
	for _, env := range []string{"PLUGINS", "PLUGIN_PINS", "PLUGIN_TOOLS"} {
		t.Setenv(brand.EnvPrefix+env, "")
	}
	t.Setenv(brand.EnvPrefix+"SANDBOX", "off")
	// Enable browser.action with a fake driver so its OnBlock wiring is covered.
	driver := filepath.Join(t.TempDir(), "browse.mjs")
	if err := os.WriteFile(driver, []byte("// test driver\n"), 0o600); err != nil {
		t.Fatalf("write fake driver: %v", err)
	}
	t.Setenv(brand.EnvPrefix+"BROWSER_ACTIONS", "1")
	t.Setenv(brand.EnvPrefix+"BROWSER_ACTION_DRIVER", driver)

	var stderr bytes.Buffer
	tools, _, _, _, err := buildTools(t.TempDir(), &stderr, warden.New(nil))
	if err != nil {
		t.Fatalf("buildTools: %v; stderr=%s", err, stderr.String())
	}

	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { j.Close() })
	wireNetguardAudit(tools, bus.New(j))

	// Every netguard-capable tool present in the map must be wired. Checking by
	// concrete type (not by key) means a future key rename fails here instead
	// of silently skipping the wiring.
	seen := map[string]bool{}
	for name, tl := range tools {
		switch tt := tl.(type) {
		case *httptool.Tool:
			seen["http"] = true
			if tt.OnBlock == nil {
				t.Errorf("%s: *httptool.Tool OnBlock not wired", name)
			}
		case *browsertool.Tool:
			seen["browser.read"] = true
			if tt.OnBlock == nil {
				t.Errorf("%s: *browser.Tool OnBlock not wired", name)
			}
		case *browsertool.ActionTool:
			seen["browser.action"] = true
			if tt.OnBlock == nil {
				t.Errorf("%s: *browser.ActionTool OnBlock not wired", name)
			}
		case *websearch.Tool:
			seen["web_search"] = true
			if tt.OnBlock == nil {
				t.Errorf("%s: *websearch.Tool OnBlock not wired", name)
			}
		}
	}
	for _, want := range []string{"http", "browser.read", "browser.action", "web_search"} {
		if !seen[want] {
			t.Errorf("expected a %s tool in the built tool map; registration key drifted?", want)
		}
	}
}
