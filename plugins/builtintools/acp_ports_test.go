// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/acpapi"
	"github.com/agezt/agezt/kernel/contract/toolregapi"
	"github.com/agezt/agezt/plugins/tools/acpagent"
)

// fakeACPCatalog is a stand-in for the catalogue port the daemon hands over.
type fakeACPCatalog struct {
	installed []string
	commands  map[string]string
}

func (c fakeACPCatalog) AnyInstalled() bool { return len(c.installed) > 0 }

func (c fakeACPCatalog) InstalledSlugs() []string { return c.installed }

func (c fakeACPCatalog) ResolveCommand(ref, fallback string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		fallback = strings.TrimSpace(fallback)
		return fallback, fallback != ""
	}
	cmd, ok := c.commands[ref]
	if !ok {
		return "", false
	}
	return cmd, true
}

// TestTheACPSpecWiresTheCataloguePort: the acp_agent tool is a plugin, so the
// daemon hands it the CONTRACT's port — and the wiring has to be real: the
// tool's answer about an unknown agent must come from the catalogue the
// daemon handed over, not from the tool's own memory.
func TestTheACPSpecWiresTheCataloguePort(t *testing.T) {
	catalog := fakeACPCatalog{installed: []string{"gemini", "codex"}}
	spec := specACPAgent()
	built, err := spec.Build(toolregapi.BuildDeps{
		Get: func(string) string { return "" }, // no default command
		ACP: catalog,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if built.Tool == nil {
		t.Fatal("an installed agent must register the tool")
	}

	raw, _ := json.Marshal(map[string]string{"task": "go", "agent": "nope"})
	res, err := built.Tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError {
		t.Fatal("an unknown agent should yield an error result")
	}
	if !strings.Contains(res.Output, "available installed agents: gemini, codex") {
		t.Errorf("the answer must come from the wired catalogue, got:\n%s", res.Output)
	}
}

// TestTheACPSpecSurvivesAMissingPort: a daemon that hands over no catalogue
// must leave the tool disabled unless a default command is configured — there
// is nothing to discover, and the tool must not call into nothing.
func TestTheACPSpecSurvivesAMissingPort(t *testing.T) {
	spec := specACPAgent()
	built, err := spec.Build(toolregapi.BuildDeps{
		Get: func(string) string { return "" }, // no default command
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if built.Tool != nil {
		t.Fatal("no catalogue and no default command must leave the tool unregistered")
	}

	// A configured default command still registers the tool — and the tool
	// then reports that it has no catalogue rather than calling into nothing.
	built, err = spec.Build(toolregapi.BuildDeps{
		Get: func(string) string { return "claude-code-acp" },
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if built.Tool == nil {
		t.Fatal("a default command must register the tool")
	}
	raw, _ := json.Marshal(map[string]string{"task": "go"})
	res, err := built.Tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "catalogue is not available") {
		t.Errorf("a tool without the catalogue must say so, got:\n%s", res.Output)
	}
}

// TestTheACPSpecUsesTheContractPort: the tool the spec builds holds the
// CONTRACT's port — the type the daemon hands over, not a module's own type.
func TestTheACPSpecUsesTheContractPort(t *testing.T) {
	var _ acpapi.Catalog = fakeACPCatalog{}
	// The tool's field is the contract's port type.
	tool := acpagent.New("cmd", "/w", fakeACPCatalog{})
	if tool == nil {
		t.Fatal("a default command must build the tool")
	}
}
