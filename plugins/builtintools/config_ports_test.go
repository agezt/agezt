// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app/configtool"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolregapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestTheConfigSpecWiresTheLivePort: the config tool is a plugin, so the
// daemon hands it the CONTRACT's port — and the wiring has to be real: an
// agent-scoped write through the tool must land in the live roster, or the
// agent would be told its override was saved while it was not.
func TestTheConfigSpecWiresTheLivePort(t *testing.T) {
	k, err := runtime.Open(withRoster(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, err := k.AddProfile(agentapi.Profile{Slug: "researcher"}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	spec := specConfig()
	built, err := spec.Build(toolregapi.BuildDeps{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolregapi.KernelDeps{Config: configtool.NewKernelSource(configtool.PortsFromKernel(k))}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	ctx := toolapi.WithAgent(context.Background(), "researcher")
	raw, _ := json.Marshal(map[string]any{"op": "set", "scope": "agent", "name": "AGEZT_MODEL", "value": "agent-next"})
	res, err := built.Tool.Invoke(ctx, raw)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the agent-scoped write failed: %s", res.Output)
	}
	stored, ok := k.Roster().Get("researcher")
	if !ok || stored.ConfigOverrides["AGEZT_MODEL"] != "agent-next" {
		t.Fatalf("the tool did not write the live roster: %+v ok=%v", stored.ConfigOverrides, ok)
	}
}

// TestTheConfigSpecSurvivesAMissingPort: a daemon that handed over only the
// kernel must leave the agent-scoped write reporting that it needs a live
// daemon — not dereferencing nil.
func TestTheConfigSpecSurvivesAMissingPort(t *testing.T) {
	k, err := runtime.Open(withRoster(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	spec := specConfig()
	built, err := spec.Build(toolregapi.BuildDeps{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolregapi.KernelDeps{}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	ctx := toolapi.WithAgent(context.Background(), "researcher")
	raw, _ := json.Marshal(map[string]any{"op": "set", "scope": "agent", "name": "AGEZT_MODEL", "value": "agent-next"})
	res, err := built.Tool.Invoke(ctx, raw)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "live kernel") {
		t.Fatalf("an unwired port must report that, got: %s", res.Output)
	}
}
