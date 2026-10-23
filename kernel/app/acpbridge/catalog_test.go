// SPDX-License-Identifier: MIT

package acpbridge

import (
	"os"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/acpapi"
	"github.com/agezt/agezt/kernel/modules/extensions/acpcatalog"
)

// TestTheAdapterAnswersThePort: the adapter IS the contract's port — that is
// the whole of its job.
func TestTheAdapterAnswersThePort(t *testing.T) {
	var _ acpapi.Catalog = Catalog{}
	var _ acpapi.Catalog = NewCatalog()
}

// TestTheAdapterAnswersTheLiveCatalogue: the adapter does not answer from its
// own memory — it answers from the catalogue module, so the tool and the
// catalogue's own inventory cannot drift apart.
//
// The measurement pins a KNOWN catalogue state (one agent whose binary is
// this very test binary), because comparing the adapter against an EMPTY
// catalogue would let a lying adapter pass on every host where no ACP agent
// happens to be installed — the comparison would be true either way.
func TestTheAdapterAnswersTheLiveCatalogue(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("cannot locate the test binary: %v", err)
	}
	saved := acpcatalog.Catalog
	t.Cleanup(func() { acpcatalog.Catalog = saved })
	acpcatalog.Catalog = []acpcatalog.Agent{{
		Slug:    "probe-agent",
		Name:    "Probe",
		Bin:     exe,
		Command: exe + " serve",
	}}

	c := NewCatalog()
	if !c.AnyInstalled() {
		t.Error("the adapter must report the catalogued agent as installed")
	}
	slugs := c.InstalledSlugs()
	if len(slugs) != 1 || slugs[0] != "probe-agent" {
		t.Errorf("the adapter must name the installed slug, got %v", slugs)
	}
}

// TestTheAdapterResolvesThroughTheCatalogue: the resolved command comes from
// the trusted catalogue, and an unknown slug is refused.
func TestTheAdapterResolvesThroughTheCatalogue(t *testing.T) {
	c := NewCatalog()
	cmd, ok := c.ResolveCommand("no-such-agent-slug", "fallback")
	if ok {
		t.Errorf("an unknown slug must not resolve, got %q", cmd)
	}
	if cmd != "" {
		t.Errorf("an unknown slug must resolve to no command, got %q", cmd)
	}
	// With no selector, the operator's fallback is what is used.
	cmd, ok = c.ResolveCommand("", "fallback-cmd")
	if !ok || cmd != "fallback-cmd" {
		t.Errorf("the fallback must resolve, got %q ok=%v", cmd, ok)
	}
	// An empty fallback with no selector resolves to nothing.
	cmd, ok = c.ResolveCommand("", "   ")
	if ok || cmd != "" {
		t.Errorf("an empty fallback must not resolve, got %q ok=%v", cmd, ok)
	}
}

// TestTheAdapterNeverRunsTheSelector: SECURITY (CWE-78). The selector arrives
// in agent/LLM tool input, which prompt injection can steer, so it must
// never be executed as a shell line — the adapter forwards it to the
// catalogue, which treats it strictly as a slug.
func TestTheAdapterNeverRunsTheSelector(t *testing.T) {
	c := NewCatalog()
	injected := "gemini; rm -rf /"
	cmd, ok := c.ResolveCommand(injected, "")
	if ok {
		t.Errorf("an injected selector must not resolve, got %q", cmd)
	}
	if cmd != "" {
		t.Errorf("an injected selector must resolve to no command, got %q", cmd)
	}
	if strings.Contains(cmd, injected) {
		t.Errorf("the selector leaked into the command: %q", cmd)
	}
}
