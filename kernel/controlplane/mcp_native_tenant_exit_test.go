// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
)

type mcpExitTenantPeer struct{}

func (mcpExitTenantPeer) Tools() []mcp.ToolDef { return nil }
func (mcpExitTenantPeer) Call(context.Context, string, json.RawMessage) (string, bool, error) {
	panic("tenant tool invocation unexpected")
}
func (mcpExitTenantPeer) Close() error { return nil }
func TestMCPNativeAllSixTenantDenialsPreserveRegistrationAttachmentAndJournal(t *testing.T) {
	p := mock.New()
	dials := 0
	k, s, _, dir := startPairWithConfig(t, runtime.Config{Provider: p, MCPDialer: func(context.Context, string, []string, map[string]string) (mcp.Conn, error) {
		dials++
		return mcpExitTenantPeer{}, nil
	}, MCPHTTPDialer: func(context.Context, string, map[string]string) (mcp.Conn, error) {
		dials++
		return mcpExitTenantPeer{}, nil
	}})
	if _, err := k.AddMCPServer("owned-seed", mcp.Server{Name: "owned", Command: "owned-never-spawned", Env: map[string]string{"OWNED": "owned-private-value"}}); err != nil {
		t.Fatal(err)
	}
	registry := withTenants(t, s, dir)
	client := tenantClient(t, dir, mustTenant(t, registry, "acme"))
	head, hash := k.Journal().Head()
	rows := k.MCPStore().List()
	attached := k.MCPAttached()
	for _, cmd := range []string{controlplane.CmdMCPList, controlplane.CmdMCPAdd, controlplane.CmdMCPAttach, controlplane.CmdMCPDetach, controlplane.CmdMCPSetEnabled, controlplane.CmdMCPRemove} {
		args := map[string]any{"ref": "owned", "enabled": false, "tenant": "acme", "server": map[string]any{"name": "new", "command": "owned-never-spawned"}}
		if _, err := client.Call(context.Background(), cmd, args); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatal(cmd, err)
		}
		after, afterHash := k.Journal().Head()
		if head != after || hash != afterHash || !reflect.DeepEqual(rows, k.MCPStore().List()) || !reflect.DeepEqual(attached, k.MCPAttached()) || dials != 0 || p.CallCount() != 0 {
			t.Fatal(cmd, head, after, dials, p.CallCount())
		}
	}
}
