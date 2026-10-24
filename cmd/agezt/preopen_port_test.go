// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolregapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/warden"
)

// stubScriptRunner is a script runner: the contract's one verb.
type stubScriptRunner struct{}

func (stubScriptRunner) RunScript(ctx context.Context, language, code, inputJSON string) (string, bool, error) {
	return "ok", false, nil
}

// TestThePreOpenPortInstallsTheRunner: the daemon's adapter is the ONE place
// that knows both the contract's port and the daemon's configuration, and it
// must actually install the runner.
//
// The installation is the effect the script-tool forge (M794) depends on:
// without it the forge reports itself unavailable and the agent cannot test
// the tools it authors. The adapter used to live inside the registry; it
// lives HERE now (W3.39h) because the registry moved to the contract layer
// and may not name the daemon's configuration at all.
func TestThePreOpenPortInstallsTheRunner(t *testing.T) {
	var cfg runtime.Config
	if cfg.ScriptRunner != nil {
		t.Fatal("a fresh configuration must carry no script runner")
	}
	port := runtimeConfigPort{cfg: &cfg}
	port.SetScriptRunner(stubScriptRunner{})
	if cfg.ScriptRunner == nil {
		t.Fatal("the port did not install the script runner into the configuration")
	}
}

// TestThePreOpenPortRefusesAMissingConfiguration: a configuration that is not
// there is REFUSED rather than dereferenced.
//
// A nil configuration would otherwise panic inside the port — and the panic
// would land in the pre-Open phase, before the kernel opens, where the
// operator sees a crash instead of a tool that is not wired.
func TestThePreOpenPortRefusesAMissingConfiguration(t *testing.T) {
	port := runtimeConfigPort{cfg: nil}
	port.SetScriptRunner(stubScriptRunner{}) // must not panic
}

// probePort is the port the probe spec's pre-Open hook received. It is a
// package-level cell because the registry is global and a spec's hook has no
// other way to hand something back to the test that registered it.
var probePort toolregapi.Config

// probeTool is the minimal tool the probe spec builds. It registers under its
// own name and never collides with a real tool.
type probeTool struct{}

func (probeTool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{Name: "preopen_probe"}
}

func (probeTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{}, nil
}

// TestApplyPreOpenHandsTheHookTheDaemonsPort: the daemon's pre-Open wiring
// is a CONTRACT with the tools, and this is the call site that keeps it.
//
// The registry's phase used to wrap the daemon's configuration itself; now the
// registry lives in the contract layer and may not name that type at all, so
// the COMPOSITION ROOT builds the port and hands it over. If the daemon handed
// over something else — a nil port, a port over a copy — the script-tool
// forge would keep reporting itself unavailable (M794), and nothing anywhere
// would say so.
//
// The probe spec is registered under its own name and builds a tool of its
// own, so it cannot collide with a real one; it is the only spec whose
// pre-Open hook answers, and it records the port it was given.
func TestApplyPreOpenHandsTheHookTheDaemonsPort(t *testing.T) {
	toolregapi.Register(toolregapi.Spec{
		Name: "preopen_probe",
		Build: func(toolregapi.BuildDeps) (toolregapi.Built, error) {
			return toolregapi.Built{Tool: probeTool{}}, nil
		},
		PreOpen: func(_ toolapi.Tool, cfg toolregapi.Config) { probePort = cfg },
	})

	var stderr bytes.Buffer
	_, set, _, _, _, err := buildTools(t.TempDir(), &stderr, warden.New(nil), nil)
	if err != nil {
		t.Fatalf("buildTools: %v; stderr=%s", err, stderr.String())
	}
	defer set.Close()

	probePort = nil
	var cfg runtime.Config
	applyPreOpen(set, &cfg)

	if probePort == nil {
		t.Fatal("the pre-Open phase never reached a spec's hook")
	}
	// The port the phase handed over is wired to THIS configuration: what is
	// installed through it lands in the daemon's configuration.
	probePort.SetScriptRunner(stubScriptRunner{})
	if cfg.ScriptRunner == nil {
		t.Fatal("the port the phase handed over is not wired to the daemon's configuration")
	}
}
