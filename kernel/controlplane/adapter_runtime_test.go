// SPDX-License-Identifier: MIT

package controlplane

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The control plane is an ADAPTER: it speaks the protocol and hands the work
// to the application layer. It must not reach into the runtime's module for
// the run's own DECORATION — the agent's profile, the wake provenance, the
// cost ceiling, the model chain, the tool allowlist.
//
// Those are decisions the APPLICATION makes (`kernel/app/runs.Decorate` and
// its siblings): the host states WHY a run exists and WHICH agent runs it,
// and it says so without naming the module that will execute the run.
//
// The GATE holds the adapter→module edge at PACKAGE granularity: as long as
// the adapter names the kernel handle at all, the edge is allowed and the gate
// says nothing about WHICH symbols the adapter names beside it. This test
// holds the same edge at SYMBOL granularity — the decoration symbols are the
// ones that turn "the adapter holds a kernel" into "the adapter knows how the
// runtime builds a run".
var runtimeSelector = regexp.MustCompile(`\b(?:kernel)?runtime\.([A-Za-z_][A-Za-z0-9_]*)`)

func TestTheAdapterNamesNoRunDecorationFromTheRuntime(t *testing.T) {
	forbidden := map[string]bool{
		"WithAgentProfile": true,
		"WithWakeContext":  true,
		"WithMaxCost":      true,
		"WithModel":        true,
		"WithModelChain":   true,
		"WithTools":        true,
		"WakeContext":      true,
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range runtimeSelector.FindAllStringSubmatch(string(raw), -1) {
			if forbidden[m[1]] {
				t.Errorf("%s names runtime.%s — the run's decoration belongs to the application layer (kernel/app/runs)", name, m[1])
			}
		}
	}
}
