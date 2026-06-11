// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// wireMCPConn is a canned attachment for the wire round-trip.
type wireMCPConn struct{ closed bool }

func (c *wireMCPConn) Tools() []mcp.ToolDef {
	return []mcp.ToolDef{{Name: "greet", Description: "greets"}}
}
func (c *wireMCPConn) Call(context.Context, string, json.RawMessage) (string, bool, error) {
	return "hello", false, nil
}
func (c *wireMCPConn) Close() error { c.closed = true; return nil }

// TestMCP_WireRoundTrip drives the full operator pipeline over the wire:
// add → attach (discovered tool names returned, list shows live status) →
// disable auto-attach → detach → remove. Exactly what `agt mcp` and the
// console speak.
func TestMCP_WireRoundTrip(t *testing.T) {
	conn := &wireMCPConn{}
	_, _, c, _ := startPairWithConfig(t, runtime.Config{
		Provider: mock.New(mock.FinalText("unused")),
		MCPDialer: func(context.Context, string, []string, map[string]string) (mcp.Conn, error) {
			return conn, nil
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Add.
	res, err := c.Call(ctx, controlplane.CmdMCPAdd, map[string]any{
		"server": map[string]any{"name": "fake", "command": "python", "args": []any{"server.py"}},
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	srv, _ := res["server"].(map[string]any)
	if srv["attached"] != false || srv["enabled"] != true {
		t.Fatalf("added = %v", srv)
	}

	// Attach → tool names come back, list shows it live.
	res, err = c.Call(ctx, controlplane.CmdMCPAttach, map[string]any{"ref": "fake"})
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	tools, _ := res["tools"].([]any)
	if len(tools) != 1 || tools[0] != "mcp_fake_greet" {
		t.Fatalf("attach tools = %v", tools)
	}
	res, err = c.Call(ctx, controlplane.CmdMCPList, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if ac, _ := res["attached_count"].(float64); ac != 1 {
		t.Fatalf("attached_count = %v", res["attached_count"])
	}

	// Disable auto-attach (registration survives, attachment untouched).
	res, err = c.Call(ctx, controlplane.CmdMCPSetEnabled, map[string]any{"ref": "fake", "enabled": false})
	if err != nil {
		t.Fatalf("set_enabled: %v", err)
	}
	srv, _ = res["server"].(map[string]any)
	if srv["enabled"] != false || srv["attached"] != true {
		t.Fatalf("disabled = %v", srv)
	}

	// Detach (kill switch).
	if _, err = c.Call(ctx, controlplane.CmdMCPDetach, map[string]any{"ref": "fake"}); err != nil {
		t.Fatalf("detach: %v", err)
	}
	if !conn.closed {
		t.Fatal("detach did not close the connection")
	}

	// Remove.
	res, err = c.Call(ctx, controlplane.CmdMCPRemove, map[string]any{"ref": "fake"})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed, _ := res["removed"].(bool); !removed {
		t.Fatal("remove reported false")
	}

	// Ghost refs are honest errors.
	if _, err := c.Call(ctx, controlplane.CmdMCPAttach, map[string]any{"ref": "ghost"}); err == nil {
		t.Fatal("ghost attach accepted")
	}
}

// TestMCP_RemoteServerViewRedactsHeaders (M904, #39): a remote (URL)
// registration round-trips with a transport badge and its header VALUES
// redacted — only sorted key names are exposed, like env.
func TestMCP_RemoteServerViewRedactsHeaders(t *testing.T) {
	_, _, c, _ := startPairWithConfig(t, runtime.Config{
		Provider: mock.New(mock.FinalText("unused")),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := c.Call(ctx, controlplane.CmdMCPAdd, map[string]any{
		"server": map[string]any{
			"name": "remote",
			"url":  "https://mcp.example.com/v1",
			"headers": map[string]any{
				"Authorization": "Bearer super-secret",
				"X-Trace-Id":    "abc",
			},
		},
	})
	if err != nil {
		t.Fatalf("add remote: %v", err)
	}
	srv, _ := res["server"].(map[string]any)
	if srv["transport"] != "http" {
		t.Errorf("transport = %v, want http", srv["transport"])
	}
	if _, leaked := srv["headers"]; leaked {
		t.Error("raw header values leaked over the wire")
	}
	keys, _ := srv["header_keys"].([]any)
	if len(keys) != 2 || keys[0] != "Authorization" || keys[1] != "X-Trace-Id" {
		t.Errorf("header_keys = %v, want sorted [Authorization X-Trace-Id]", keys)
	}
	// And the secret value must appear nowhere in the serialized view.
	blob, _ := json.Marshal(srv)
	if strings.Contains(string(blob), "super-secret") {
		t.Error("serialized view contained the header secret")
	}
}
