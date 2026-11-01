// SPDX-License-Identifier: MIT

package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The PROMPT ASSEMBLY moved to the runs module in W3.76e: the layer that
// decides what a run knows before it starts — the live identity, the operator
// profile, the taste exemplars, the recalled memory, the resolved world
// entities, the activated skills — plus the host-environment preamble.
//
// The daemon keeps the DOORS (BuildRunPrompt, InjectHostEnvironment) and the
// three-verb host port (the live identity, the bus, the clock). It must not
// grow a second copy of the assembly.
//
// These tests are the ratchet.

// TestThePromptAssemblyLivesInTheRunsModule pins the move itself: the
// daemon's prompt files are GONE and the module carries the family.
func TestThePromptAssemblyLivesInTheRunsModule(t *testing.T) {
	for _, gone := range []string{
		"prompt_run.go",
		"prompt_context.go",
		"prompt_environment.go",
		"prompt_env.go",
		"subagent_inject.go",
	} {
		if _, err := os.Stat(filepath.Join(".", gone)); err == nil {
			t.Fatalf("the daemon kept %s; the prompt assembly is the module's", gone)
		}
	}
	module := "../modules/runs/prompt"
	for _, want := range []string{
		"doc.go",
		"config.go",
		"service.go",
		"inject.go",
		"environment.go",
		"selection.go",
	} {
		if _, err := os.Stat(filepath.Join(module, want)); err != nil {
			t.Fatalf("the runs module must carry %s: %v", want, err)
		}
	}
}

// TestTheDaemonHoldsNoPromptBody is the ratchet on the daemon's side: the
// assembly's bodies must not come back under a new name. The daemon's own
// file is skipped — the list below lives in it.
func TestTheDaemonHoldsNoPromptBody(t *testing.T) {
	forbidden := []string{
		"func (k *Kernel) buildRunPrompt",
		"func (k *Kernel) injectHostEnvironment",
		"func (k *Kernel) subAgentInjectedSystem",
		"func (k *Kernel) publishContextSelection",
		"func injectMemory(",
		"func injectTaste(",
		"func injectWorld(",
		"func injectSkills(",
		"func injectUserProfile(",
		"func injectEnvironment(",
		"func capabilityBriefing(",
		"func forgeBias(",
		"func memoryContextCandidates(",
		"func worldContextCandidates(",
		"func skillContextCandidates(",
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the daemon's directory: %v", err)
	}
	self := "prompt_ratchet_test.go"
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == self {
			continue
		}
		b, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		body := string(b)
		for _, bad := range forbidden {
			if strings.Contains(body, bad) {
				t.Errorf("%s carries %q; the prompt assembly is the module's", name, bad)
			}
		}
	}
}

// TestTheDaemonKeepsThePromptDoors pins the other half of the contract: the
// daemon DOES keep the doors, and they live in the one file that is allowed
// to name the module's service.
func TestTheDaemonKeepsThePromptDoors(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(".", "prompt_doors.go"))
	if err != nil {
		t.Fatalf("the daemon must keep its prompt doors: %v", err)
	}
	body := string(b)
	for _, want := range []string{
		"func (k *Kernel) BuildRunPrompt(",
		"func (k *Kernel) InjectHostEnvironment(",
		"k.prompt.Build(",
		"k.prompt.BuildEnvironment(",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("prompt_doors.go must carry %q", want)
		}
	}
}

// TestTheDaemonHoldsThePromptService pins the composition: the kernel holds
// the module's service and the host port that feeds it.
func TestTheDaemonHoldsThePromptService(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(".", "runtime_kernel.go"))
	if err != nil {
		t.Fatalf("read the kernel struct: %v", err)
	}
	if !strings.Contains(string(b), "prompt *prompt.Service") {
		t.Error("the kernel must hold the runs module's prompt service")
	}
}
