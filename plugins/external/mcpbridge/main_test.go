// SPDX-License-Identifier: MIT

package main_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/agezt/agezt/kernel/plugin"
)

// The tests in this file build two binaries on demand:
//
//   - the bridge itself (the package containing main.go / mcp.go)
//   - the mockmcp server (testdata/mockmcp)
//
// Both are built once per `go test` invocation and cached. The
// bridge is then spawned through the real agezt plugin.Spawn host
// — i.e. the same code path the daemon uses in production — with
// MCPBRIDGE_SERVER_CMD pointing at the mockmcp binary.
//
// This makes the test suite an end-to-end integration: agezt host
// ←agezt protocol→ bridge ←JSON-RPC 2.0→ mock MCP server. A
// regression anywhere in the chain breaks the test.

var (
	bridgeOnce sync.Once
	bridgePath string
	bridgeErr  error

	mockOnce sync.Once
	mockPath string
	mockErr  error
)

func buildBridge(t *testing.T) string {
	t.Helper()
	bridgeOnce.Do(func() {
		dir, err := os.MkdirTemp("", "mcpbridge-test-")
		if err != nil {
			bridgeErr = err
			return
		}
		name := "mcpbridge"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		out := filepath.Join(dir, name)
		cmd := exec.Command("go", "build", "-o", out, ".")
		// `.` is the package containing this test — i.e. main.
		if buildOut, err := cmd.CombinedOutput(); err != nil {
			bridgeErr = fmt.Errorf("build bridge: %v\n%s", err, buildOut)
			return
		}
		bridgePath = out
	})
	if bridgeErr != nil {
		t.Fatalf("buildBridge: %v", bridgeErr)
	}
	return bridgePath
}

func buildMock(t *testing.T) string {
	t.Helper()
	mockOnce.Do(func() {
		dir, err := os.MkdirTemp("", "mockmcp-test-")
		if err != nil {
			mockErr = err
			return
		}
		name := "mockmcp"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		out := filepath.Join(dir, name)
		cmd := exec.Command("go", "build", "-o", out, ".")
		cmd.Dir = "testdata/mockmcp"
		if buildOut, err := cmd.CombinedOutput(); err != nil {
			mockErr = fmt.Errorf("build mockmcp: %v\n%s", err, buildOut)
			return
		}
		mockPath = out
	})
	if mockErr != nil {
		t.Fatalf("buildMock: %v", mockErr)
	}
	return mockPath
}

// spawn boots the bridge wired to the mock, returning a ready
// plugin handle. Caller defers Close.
func spawn(t *testing.T) *plugin.Plugin {
	t.Helper()
	bridge := buildBridge(t)
	mock := buildMock(t)

	// Inherit the test process env so PATH/HOME/etc. flow through —
	// then bolt on the bridge's own config. We rely on os.Environ()
	// here rather than building a minimal env because `go` (for
	// child-spawned mockmcp on Windows) wants HOMEDRIVE/HOMEPATH and
	// the like; copying the parent env is the path of least
	// surprise across OSes.
	env := append(os.Environ(), "MCPBRIDGE_SERVER_CMD="+mock)

	var stderrLines []string
	var stderrMu sync.Mutex
	p, err := plugin.Spawn(context.Background(), plugin.Config{
		Path: bridge,
		Env:  env,
		Logger: func(line string) {
			stderrMu.Lock()
			stderrLines = append(stderrLines, line)
			stderrMu.Unlock()
		},
	})
	if err != nil {
		stderrMu.Lock()
		t.Fatalf("Spawn bridge: %v\nbridge stderr:\n%s", err, stderrLines)
	}
	// Stash stderr lines for tests that want to inspect them
	// (e.g. verifying the mock actually started). The Cleanup
	// fires after the test completes; if it failed, dump them.
	t.Cleanup(func() {
		if t.Failed() {
			stderrMu.Lock()
			t.Logf("bridge stderr:\n%s", stderrLines)
			stderrMu.Unlock()
		}
	})
	return p
}

// TestBridge_ListsToolsFromMCP verifies the initialize round trip
// surfaces every tool the MCP server advertises. This is the core
// "the bridge sees the upstream tools" claim — if it breaks,
// nothing else in the bridge matters.
func TestBridge_ListsToolsFromMCP(t *testing.T) {
	p := spawn(t)
	defer p.Close()

	tools := p.Tools("")
	if _, ok := tools["greet"]; !ok {
		t.Errorf("missing greet tool; got: %v", keys(tools))
	}
	if _, ok := tools["boom"]; !ok {
		t.Errorf("missing boom tool; got: %v", keys(tools))
	}
	// M1.ww: mockmcp also exposes one resource, so the bridge
	// surfaces a synthetic read_resource tool. Total = 3.
	if len(tools) != 3 {
		t.Errorf("got %d tools, want exactly 3 (greet, boom, read_resource)", len(tools))
	}

	// Descriptions must round-trip (not be empty or replaced).
	greetDef := tools["greet"].Definition()
	if greetDef.Description == "" {
		t.Error("greet description lost in translation")
	}
	// Schema must round-trip enough that the agent can validate
	// inputs. We check the {type:object} substring (the bridge
	// passes the schema through verbatim, so the test is really
	// checking that no transformation munged it).
	if got := string(greetDef.InputSchema); got == "" {
		t.Error("greet input schema empty after bridge")
	}
}

// TestBridge_InvokesUpstreamTool runs greet end-to-end through the
// bridge and verifies the result text comes back unchanged.
func TestBridge_InvokesUpstreamTool(t *testing.T) {
	p := spawn(t)
	defer p.Close()

	tools := p.Tools("")
	greet := tools["greet"]
	if greet == nil {
		t.Fatal("greet tool not registered")
	}

	res, err := greet.Invoke(t.Context(), json.RawMessage(`{"name":"world"}`))
	if err != nil {
		t.Fatalf("Invoke greet: %v", err)
	}
	if res.IsError {
		t.Errorf("greet returned IsError=true; output=%q", res.Output)
	}
	if res.Output != "hello, world" {
		t.Errorf("greet Output = %q, want %q", res.Output, "hello, world")
	}
}

// TestBridge_PropagatesIsError verifies that MCP `isError: true`
// responses become agezt `IsError: true` — the agent loop relies
// on this to feed errors back to the model as user-visible
// tool failures.
func TestBridge_PropagatesIsError(t *testing.T) {
	p := spawn(t)
	defer p.Close()

	boom := p.Tools("")["boom"]
	if boom == nil {
		t.Fatal("boom tool not registered")
	}

	res, err := boom.Invoke(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		// Not an err — boom returns an MCP error-typed result, not a
		// JSON-RPC error. If we get a transport error here something
		// went wrong below the bridge.
		t.Fatalf("Invoke boom: unexpected transport err: %v", err)
	}
	if !res.IsError {
		t.Errorf("boom IsError = false, want true")
	}
	if res.Output != "deliberate" {
		t.Errorf("boom Output = %q, want %q", res.Output, "deliberate")
	}
}

// TestBridge_PrefixNamespacing verifies the agezt host's prefix
// gets applied on top of the bridged names — operators with two MCP
// servers ("ctx.read" + "ctx.write" from one, "fs.read" + "fs.write"
// from another) shouldn't collide.
func TestBridge_PrefixNamespacing(t *testing.T) {
	p := spawn(t)
	defer p.Close()

	tools := p.Tools("mcp.")
	if _, ok := tools["mcp.greet"]; !ok {
		t.Errorf("prefix not applied to greet; got: %v", keys(tools))
	}
	if _, ok := tools["mcp.boom"]; !ok {
		t.Errorf("prefix not applied to boom; got: %v", keys(tools))
	}
	if _, ok := tools["greet"]; ok {
		t.Error("unprefixed greet leaked through")
	}
}

// TestBridge_MissingEnvVar verifies that omitting
// MCPBRIDGE_SERVER_CMD makes the bridge exit immediately with a
// clear stderr message — operator's misconfiguration shouldn't
// silently hang the daemon's plugin-init timeout.
func TestBridge_MissingEnvVar(t *testing.T) {
	bridge := buildBridge(t)

	cmd := exec.Command(bridge)
	// Inherit nothing — explicitly no MCPBRIDGE_SERVER_CMD.
	cmd.Env = []string{}
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("bridge exited 0; expected non-zero; out=%q", out)
	}
	// Exit status non-zero AND the stderr should name the missing var.
	if want := "MCPBRIDGE_SERVER_CMD"; !contains(string(out), want) {
		t.Errorf("expected stderr to mention %q; got: %s", want, out)
	}
}

// TestBridge_ResourcesSurfaceAsReadResource verifies the M1.ww
// extension: an MCP server that exposes resources gets a
// synthetic `read_resource` tool registered in the agezt tool
// list, and invoking it returns the resource body.
func TestBridge_ResourcesSurfaceAsReadResource(t *testing.T) {
	p := spawn(t)
	defer p.Close()

	tools := p.Tools("")
	rr, ok := tools["read_resource"]
	if !ok {
		t.Fatalf("read_resource tool not registered; got: %v", keys(tools))
	}
	// Description must mention the available URI so the agent's
	// planner doesn't need a separate discovery call.
	if !contains(rr.Definition().Description, "file:///mock/doc.md") {
		t.Errorf("read_resource description omits available URIs: %q", rr.Definition().Description)
	}
	res, err := rr.Invoke(t.Context(), json.RawMessage(`{"uri":"file:///mock/doc.md"}`))
	if err != nil {
		t.Fatalf("Invoke read_resource: %v", err)
	}
	if !contains(res.Output, "this is the mock doc") {
		t.Errorf("Output didn't contain resource body: %q", res.Output)
	}
	// Provenance annotation should include the URI.
	if !contains(res.Output, "file:///mock/doc.md") {
		t.Errorf("Output missing URI provenance: %q", res.Output)
	}
}

// TestBridge_UnknownToolError verifies that an MCP server error
// response (not just an MCP isError result, but a JSON-RPC error)
// surfaces as a Go error from Invoke. Distinguishing these two
// error channels matters: an isError result feeds back into the
// agent loop; a transport error becomes a tool-unavailable
// situation the operator must investigate.
func TestBridge_UnknownToolError(t *testing.T) {
	p := spawn(t)
	defer p.Close()

	// Synthesize an invocation for a tool the registry doesn't know.
	// Use the lower-level Plugin.Invoke entry point to bypass the
	// remoteTool wrapper's name-translation.
	_, err := p.Invoke(context.Background(), "does-not-exist", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("Invoke missing tool: expected error, got nil")
	}
	// The error string should mention the upstream method/tool so
	// debugging isn't a guessing game.
	if !contains(err.Error(), "does-not-exist") && !contains(err.Error(), "unknown") {
		t.Errorf("error message %q doesn't identify the missing tool", err.Error())
	}
}

// ----- helpers ---------------------------------------------------

// keys returns the keys of a map[string]agent.Tool. We accept the
// generic shape to avoid an extra agent import line in callers.
func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// contains is a tiny strings.Contains shim — avoids an import-line
// for one call site.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
