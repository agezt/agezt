// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app/introspect"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolregapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestIntrospectSpecReadsTheLiveKernelThroughThePort: the introspect tool is
// a plugin, so it may not name the kernel — the daemon hands over the
// app-layer adapter through the contract's port. The wiring has to be real:
// the tool must publish the LIVE kernel's state, not a stale copy.
func TestIntrospectSpecReadsTheLiveKernelThroughThePort(t *testing.T) {
	k, err := runtime.Open(withMemory(t, withWorld(t, runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
		Tools:    map[string]toolapi.Tool{"probe": nil},
	})))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	spec := specIntrospect()
	built, err := spec.Build(toolregapi.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolregapi.KernelDeps{
		Introspect: introspect.NewKernelSource(introspect.PortsFromKernel(k)),
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"overview"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the introspect tool errored: %s", res.Output)
	}
	// The tool's answer is the live kernel's own state — the daemon version
	// and the registered tool names come from the kernel the daemon opened.
	if !strings.Contains(res.Output, `"daemon"`) {
		t.Fatalf("the overview must carry the daemon row: %s", res.Output)
	}
	var wire struct {
		Daemon string   `json:"daemon"`
		Tools  []string `json:"tools"`
	}
	if err := json.Unmarshal([]byte(res.Output), &wire); err != nil {
		t.Fatalf("the overview is not JSON: %v (%s)", err, res.Output)
	}
	if wire.Daemon == "" {
		t.Fatalf("the overview must name the daemon: %s", res.Output)
	}
	if len(wire.Tools) == 0 {
		t.Fatalf("the overview must list the live kernel's tools: %s", res.Output)
	}
}

// TestIntrospectSpecSurvivesAnUnwiredPort: the Configure hook runs for every
// daemon — one that does not supply the port must leave the tool unbound, and
// the tool must then say so instead of dereferencing nil.
func TestIntrospectSpecSurvivesAnUnwiredPort(t *testing.T) {
	spec := specIntrospect()
	built, err := spec.Build(toolregapi.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolregapi.KernelDeps{}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"overview"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError {
		t.Fatalf("an unwired introspect tool must report that, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "not available") {
		t.Fatalf("the unwired tool must say it is not available: %s", res.Output)
	}
}
