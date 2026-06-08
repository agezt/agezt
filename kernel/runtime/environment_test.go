// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"

	"encoding/json"

	"github.com/agezt/agezt/kernel/agent"
	kruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// fileLikeTool is a stand-in named "file" so the preamble's tool list has a
// predictable entry to assert on.
type fileLikeTool struct{}

func (fileLikeTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "file", Description: "Read and write files. Extra detail.", InputSchema: json.RawMessage(`{"type":"object"}`)}
}
func (fileLikeTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	return agent.Result{Output: "ok"}, nil
}

// TestEnvironmentInject_ReachesSystemPrompt: with EnvironmentInject on, the
// host-environment preamble (OS, workspace, tools) is prepended to the system
// prompt the provider actually receives, ahead of the configured persona (M609).
func TestEnvironmentInject_ReachesSystemPrompt(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"))
	var system string
	var mu sync.Mutex
	prov.OnRequest = func(req agent.CompletionRequest) {
		mu.Lock()
		system = req.System
		mu.Unlock()
	}

	k, err := kruntime.Open(kruntime.Config{
		BaseDir:           t.TempDir(),
		Provider:          prov,
		Tools:             map[string]agent.Tool{"file": fileLikeTool{}},
		System:            "YOU ARE A HELPFUL PERSONA",
		EnvironmentInject: true,
		WorkspaceRoot:     `/tmp/ws-test`,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Run(context.Background(), "do something"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()

	for _, want := range []string{
		"## Runtime environment",
		"OS / arch: " + runtime.GOOS,
		"/tmp/ws-test",
		"- file —",
		"YOU ARE A HELPFUL PERSONA",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system prompt missing %q\n---\n%s", want, system)
		}
	}
	// Preamble precedes the persona.
	if strings.Index(system, "Runtime environment") > strings.Index(system, "HELPFUL PERSONA") {
		t.Error("environment preamble should come before the configured persona")
	}
}

// TestEnvironmentInject_OffByConfig: with EnvironmentInject false the preamble
// is absent and the persona is sent verbatim.
func TestEnvironmentInject_OffByConfig(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"))
	var system string
	prov.OnRequest = func(req agent.CompletionRequest) { system = req.System }

	k, err := kruntime.Open(kruntime.Config{
		BaseDir:           t.TempDir(),
		Provider:          prov,
		System:            "PERSONA ONLY",
		EnvironmentInject: false,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	if _, _, err := k.Run(context.Background(), "hi"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(system, "Runtime environment") {
		t.Errorf("preamble must be absent when EnvironmentInject is off:\n%s", system)
	}
	if system != "PERSONA ONLY" {
		t.Errorf("persona = %q want verbatim 'PERSONA ONLY'", system)
	}
}
