// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/toolreg"
	mcptool "github.com/agezt/agezt/plugins/tools/mcptool"
	standingtool "github.com/agezt/agezt/plugins/tools/standingtool"
)

// fakeMCPRunner is the daemon's MCP registry, faked: it answers the seven
// verbs of the contract's port and RECORDS that it was called.
//
// The recording is the point: "the tool did not complain" is not enough — a
// tool could report unavailability for another reason and still look wired.
type fakeMCPRunner struct {
	listed bool
}

func (f *fakeMCPRunner) AddMCPServer(_ string, srv mcpapi.Server) (mcpapi.Server, error) {
	return srv, nil
}

func (f *fakeMCPRunner) AttachMCPServer(_ context.Context, _, _ string) (mcpapi.Server, []string, error) {
	return mcpapi.Server{}, nil, nil
}

func (f *fakeMCPRunner) DetachMCPServer(_, _ string) error { return nil }

func (f *fakeMCPRunner) RemoveMCPServer(_, _ string) (bool, error) { return true, nil }

func (f *fakeMCPRunner) MCPServers() []mcpapi.Server {
	f.listed = true
	return nil
}

func (f *fakeMCPRunner) MCPServer(string) (mcpapi.Server, bool) { return mcpapi.Server{}, false }

func (f *fakeMCPRunner) MCPAttached() map[string]int { return nil }

// fakeStandingHost is the daemon's standing orders, faked: it answers the
// three verbs of the contract's port and records the calls.
type fakeStandingHost struct {
	listed bool
}

func (f *fakeStandingHost) AddStanding(o standingapi.Order) (standingapi.Order, error) {
	return o, nil
}

func (f *fakeStandingHost) RemoveStanding(string) (bool, error) { return true, nil }

func (f *fakeStandingHost) StandingList() []standingapi.Order {
	f.listed = true
	return nil
}

// TestTheMCPSpecBindsTheRunnerPort: the MCP tool drives the daemon's MCP
// registry through the contract's port, and the registry's row carries that
// port (W3.39d) — not the live kernel.
func TestTheMCPSpecBindsTheRunnerPort(t *testing.T) {
	fake := &fakeMCPRunner{}
	spec := specMCP()
	built, err := spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolreg.KernelDeps{MCP: fake}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the mcp tool errored on a wired runner port: %s", res.Output)
	}
	if !fake.listed {
		t.Fatal("the mcp tool must reach the daemon's mcp registry")
	}
}

// TestTheStandingSpecBindsTheHostPort: the standing-orders tool drives the
// daemon's orders through the contract's port, and the roster lookup arrives
// as a plain closure (W3.39d).
func TestTheStandingSpecBindsTheHostPort(t *testing.T) {
	fake := &fakeStandingHost{}
	lookup := func(ref string) (agentapi.Profile, bool) {
		return agentapi.Profile{Slug: ref}, true
	}
	spec := specStanding()
	built, err := spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolreg.KernelDeps{Standing: fake, AgentLookup: lookup}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the standing tool errored on a wired host port: %s", res.Output)
	}
	if !fake.listed {
		t.Fatal("the standing tool must reach the daemon's standing orders")
	}
}

// TestTheRunnerlessToolsReportUnavailable: without the ports the two tools
// must say they are unavailable — they must not call into nothing.
//
// The nil port is the trap the daemon's rule exists for: a port that wraps a
// nil implementation is NOT nil, so the tool would bind a runner that is not
// there and fail in a way the operator could not read.
func TestTheMCPAndStandingRunnerlessToolsReportUnavailable(t *testing.T) {
	spec := specMCP()
	built, err := spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolreg.KernelDeps{}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("an unwired mcp registry must report that it is unavailable, got %+v", res)
	}

	spec = specStanding()
	built, err = spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolreg.KernelDeps{}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	res, err = built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("an unwired standing host must report that it is unavailable, got %+v", res)
	}
}

// TestTheToolsRunnersAreTheContractsPorts: the two tools' types ARE the
// contract's ports — the same type, not a copy with the same method set.
//
// The check is on the TYPE ITSELF, not on an assignment: an assignment would
// accept a copy (a copy has the same method set, so it satisfies the port),
// while two interface types with identical method sets are still different
// types. A tool that declared its own runner would keep working — and the
// contract's port would quietly stop being the thing the registry hands over.
func TestTheMCPAndStandingRunnersAreTheContractsPorts(t *testing.T) {
	for _, tc := range []struct {
		name string
		tool reflect.Type
		want reflect.Type
	}{
		{"mcp", reflect.TypeOf((*mcptool.Kernel)(nil)).Elem(), reflect.TypeOf((*mcpapi.Runner)(nil)).Elem()},
		{"standing", reflect.TypeOf((*standingtool.Host)(nil)).Elem(), reflect.TypeOf((*standingapi.Host)(nil)).Elem()},
	} {
		if tc.tool != tc.want {
			t.Errorf("the %s tool's runner must BE the contract's port, got %v want %v", tc.name, tc.tool, tc.want)
		}
	}
}
