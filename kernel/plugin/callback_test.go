// SPDX-License-Identifier: MIT

package plugin_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ersinkoc/agezt/kernel/agent"
	"github.com/ersinkoc/agezt/kernel/plugin"
)

// doubleTool is a tiny in-process agent.Tool the test wires into
// Plugin.Config.HostTools — it doubles the input string and returns
// it. The fixture echoplugin's "callhost" tool invokes it via the
// M1.cb host/invoke callback path.
type doubleTool struct {
	calls int
}

func (d *doubleTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name:        "double",
		Description: "Doubles the input string (test fixture for M1.cb).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`),
	}
}
func (d *doubleTool) Invoke(_ context.Context, raw json.RawMessage) (agent.Result, error) {
	d.calls++
	var args struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(raw, &args)
	return agent.Result{Output: args.Text + args.Text}, nil
}

// TestCallback_HappyPath is the end-to-end M1.cb verification:
// plugin tool 'callhost' issues a host/invoke that the host
// dispatches to the registered 'double' tool, and the doubled
// output flows back through the plugin to the original Invoke
// caller.
func TestCallback_HappyPath(t *testing.T) {
	bin := buildEchoPlugin(t)
	double := &doubleTool{}
	p, err := plugin.Spawn(context.Background(), plugin.Config{
		Path:      bin,
		HostTools: map[string]agent.Tool{"double": double},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()

	res, err := p.Invoke(context.Background(), "callhost",
		json.RawMessage(`{"text":"abc"}`))
	if err != nil {
		t.Fatalf("Invoke callhost: %v", err)
	}
	// The plugin wraps the host output as "via host: <result>".
	if !strings.Contains(res.Output, "abcabc") {
		t.Errorf("Output = %q, want it to contain doubled string abcabc", res.Output)
	}
	if double.calls != 1 {
		t.Errorf("host 'double' was invoked %d times, want 1", double.calls)
	}
}

// TestCallback_DisabledWhenHostToolsNil verifies the security
// default: with no HostTools registered, host/invoke from the
// plugin is rejected. The plugin's callhost surfaces this as an
// IsError result.
func TestCallback_DisabledWhenHostToolsNil(t *testing.T) {
	bin := buildEchoPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{
		Path: bin,
		// HostTools deliberately omitted (nil).
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()

	res, err := p.Invoke(context.Background(), "callhost",
		json.RawMessage(`{"text":"x"}`))
	if err != nil {
		// Either err or res.IsError indicates the plugin couldn't
		// complete the callback; both are acceptable failure surfaces.
		if !strings.Contains(err.Error(), "host callbacks not enabled") {
			t.Errorf("err should mention disabled callbacks: %v", err)
		}
		return
	}
	if !res.IsError && !strings.Contains(res.Output, "host callbacks not enabled") {
		t.Errorf("expected callback-disabled signal, got result=%+v", res)
	}
}

// TestCallback_ToolNotInAllowlist verifies that a HostTools map
// not containing the name the plugin asks for surfaces the
// scoped-not-found error (distinct from "callbacks blanket
// disabled"). With HostTools containing only "something_else",
// the plugin's request for "double" should fail with the
// not-in-allowlist error.
func TestCallback_ToolNotInAllowlist(t *testing.T) {
	bin := buildEchoPlugin(t)
	other := &doubleTool{}
	p, err := plugin.Spawn(context.Background(), plugin.Config{
		Path:      bin,
		HostTools: map[string]agent.Tool{"something_else": other},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()

	_, err = p.Invoke(context.Background(), "callhost",
		json.RawMessage(`{"text":"x"}`))
	if err == nil {
		t.Fatal("expected error for unknown host tool")
	}
	if !strings.Contains(err.Error(), "not in allowlist") {
		t.Errorf("err should mention allowlist: %v", err)
	}
	if other.calls != 0 {
		t.Errorf("unrelated host tool was invoked %d times, want 0", other.calls)
	}
}
