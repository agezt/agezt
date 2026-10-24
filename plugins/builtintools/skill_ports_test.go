// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolregapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestTheSkillSpecWiresTheLiveForge: the skill tool is a plugin, so it may
// not name the skills module — it asks for the CONTRACT's ports and the
// daemon hands the live implementations over. The wiring has to be real: a
// skill the agent learns must land in the kernel's own Forge, or the agent
// would learn procedures the daemon never retrieves.
func TestTheSkillSpecWiresTheLiveForge(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if k.Forge() == nil {
		t.Fatal("the kernel must own a Forge")
	}

	spec := specSkill()
	built, err := spec.Build(toolregapi.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolregapi.KernelDeps{SkillForge: k.Forge(), SkillBundles: k.Forge().Bundles()}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(
		`{"op":"learn","name":"diagnose-ci","description":"when CI fails","body":"1. read the logs"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the skill tool errored: %s", res.Output)
	}
	if !strings.Contains(res.Output, "diagnose-ci") {
		t.Fatalf("the tool did not publish the learned skill: %s", res.Output)
	}

	// The skill must be in the KERNEL's own Forge — the port really carried
	// the live implementation, not a copy of it.
	all, err := k.Forge().List()
	if err != nil {
		t.Fatalf("Forge List: %v", err)
	}
	found := false
	for _, sk := range all {
		if sk.Name == "diagnose-ci" {
			found = true
			if sk.Body != "1. read the logs" {
				t.Errorf("the stored skill's body = %q", sk.Body)
			}
			if sk.Status != "draft" {
				t.Errorf("the stored skill's status = %q, want draft", sk.Status)
			}
		}
	}
	if !found {
		t.Fatalf("the skill the tool learned did not land in the kernel's Forge: %+v", all)
	}
}

// TestTheSkillSpecHandsOverTheBundlePort: the bundle port is a SECOND wire —
// the tool reads a skill's reference files and scripts through it. If the
// daemon handed over only the Forge, the tool would answer "skill bundles are
// not available on this daemon" on every live box while every other test
// still passed.
func TestTheSkillSpecHandsOverTheBundlePort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	bundles := k.Forge().Bundles()
	if bundles == nil {
		t.Fatal("the kernel's Forge must own a bundle store")
	}

	spec := specSkill()
	built, err := spec.Build(toolregapi.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolregapi.KernelDeps{SkillForge: k.Forge(), SkillBundles: k.Forge().Bundles()}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if _, err := built.Tool.Invoke(context.Background(), json.RawMessage(
		`{"op":"learn","name":"pdf-fill","body":"fill a pdf"}`)); err != nil {
		t.Fatalf("Invoke learn: %v", err)
	}
	// Attach a bundle the way the daemon does on import.
	if _, err := bundles.Write("pdf-fill", map[string][]byte{
		"reference/fields.md": []byte("the fields"),
	}); err != nil {
		t.Fatalf("bundle Write: %v", err)
	}

	all, err := k.Forge().List()
	if err != nil {
		t.Fatalf("Forge List: %v", err)
	}
	var id string
	for _, sk := range all {
		if sk.Name == "pdf-fill" {
			id = sk.ID
		}
	}
	if id == "" {
		t.Fatalf("the learned skill is missing: %+v", all)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(
		`{"op":"files","id":"`+id+`"}`))
	if err != nil {
		t.Fatalf("Invoke files: %v", err)
	}
	if res.IsError {
		t.Fatalf("the bundle port was not handed over: %s", res.Output)
	}
	if !strings.Contains(res.Output, "reference/fields.md") {
		t.Fatalf("the tool did not list the bundle: %s", res.Output)
	}
	if !strings.Contains(res.Output, `"dir"`) {
		t.Fatalf("the tool did not report the bundle dir: %s", res.Output)
	}
}

// TestTheSkillSpecSurvivesAKernelWithoutAForge: a daemon with no Forge must
// leave the tool unbound, and the tool must then say so instead of
// dereferencing nil.
func TestTheSkillSpecSurvivesAKernelWithoutAForge(t *testing.T) {
	spec := specSkill()
	built, err := spec.Build(toolregapi.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// A kernel with no Forge: the Configure hook must not panic.
	if err := spec.Configure(built.Tool, toolregapi.KernelDeps{}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError {
		t.Fatalf("an unwired skill tool must report that, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "not available") {
		t.Fatalf("the unwired tool must say it is not available: %s", res.Output)
	}
}
