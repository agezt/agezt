// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/mcp"
	"reflect"
	"strings"
	"testing"
)

type mcpRegistrationProbe struct {
	rows  []mcp.Server
	calls int
}

func (p *mcpRegistrationProbe) List() []mcp.Server { p.calls++; return p.rows }

type mcpAttachmentProbe struct {
	snapshots []map[string]int
	calls     int
}

func (p *mcpAttachmentProbe) MCPAttached() map[string]int {
	index := p.calls
	p.calls++
	if index >= len(p.snapshots) {
		index = len(p.snapshots) - 1
	}
	return p.snapshots[index]
}
func TestMCPCatalogViewRedactsValuesSortedKeysTransportAndOptionalFields(t *testing.T) {
	rows := []mcp.Server{
		{ID: "stdio", Name: "stdio", Command: "owned command", Args: []string{" raw arg "}, Description: " raw description ", Enabled: false, Env: map[string]string{"Z_KEY": "owned-env-value", "A_KEY": "other-owned-value"}, Headers: map[string]string{"Z-Header": "owned-header-value", "A-Header": "other-owned-header"}, Lazy: true, ToolAllow: []string{" raw tool "}, CreatedMS: 10, UpdatedMS: 20},
		{ID: "http", Name: "http", URL: "http://127.0.0.1:12345/owned", Enabled: true},
		{ID: "space", Name: "space", URL: " ", Env: map[string]string{}, Headers: map[string]string{}},
	}
	for _, srv := range rows {
		host := &mcpAttachmentProbe{snapshots: []map[string]int{{srv.Name: 0}}}
		view := forgeJSON(t, NewMCPCatalog(nil, host).View(srv))
		raw, _ := json.Marshal(view)
		for _, secret := range []string{"owned-env-value", "other-owned-value", "owned-header-value", "other-owned-header"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("value leaked")
			}
		}
		for _, key := range []string{"env", "headers"} {
			if _, ok := view[key]; ok {
				t.Fatal("private map leaked", key)
			}
		}
		if view["attached"] != true || view["tool_count"] != float64(0) || host.calls != 1 {
			t.Fatal(view, host.calls)
		}
		transport := "stdio"
		if srv.URL != "" {
			transport = "http"
		}
		if view["transport"] != transport {
			t.Fatal(view)
		}
		if srv.Name == "stdio" {
			if !reflect.DeepEqual(view["env_keys"], []any{"A_KEY", "Z_KEY"}) || !reflect.DeepEqual(view["header_keys"], []any{"A-Header", "Z-Header"}) || view["command"] != srv.Command || view["description"] != srv.Description || view["lazy"] != true || !reflect.DeepEqual(view["args"], []any{" raw arg "}) || !reflect.DeepEqual(view["tool_allow"], []any{" raw tool "}) {
				t.Fatal(view)
			}
		} else {
			if _, ok := view["env_keys"]; ok {
				t.Fatal(view)
			}
			if _, ok := view["header_keys"]; ok {
				t.Fatal(view)
			}
		}
	}
}
func TestMCPCatalogViewRequiredZerosAndDetachedToolCountAbsence(t *testing.T) {
	p := &mcpAttachmentProbe{snapshots: []map[string]int{{"other": 3}}}
	view := forgeJSON(t, NewMCPCatalog(nil, p).View(mcp.Server{}))
	if len(view) != 7 {
		t.Fatal(view)
	}
	for _, key := range []string{"id", "name", "enabled", "created_ms", "updated_ms", "transport", "attached"} {
		if _, ok := view[key]; !ok {
			t.Fatal(key)
		}
	}
	if view["enabled"] != false || view["attached"] != false || view["transport"] != "stdio" || view["created_ms"] != float64(0) || view["updated_ms"] != float64(0) {
		t.Fatal(view)
	}
	if _, ok := view["tool_count"]; ok {
		t.Fatal(view)
	}
}
func TestMCPCatalogListKeepsStoreOrderFreshRowStatusAndInitialAttachedCount(t *testing.T) {
	store := &mcpRegistrationProbe{rows: []mcp.Server{{Name: "zeta"}, {Name: "alpha"}}}
	host := &mcpAttachmentProbe{snapshots: []map[string]int{{"initial": 9, "orphan": 2}, {"zeta": 0}, {"alpha": -2}}}
	out, err := NewMCPCatalog(store, host).List(context.Background(), MCPListInput{})
	if err != nil || out.Count != 2 || out.AttachedCount != 2 || store.calls != 1 || host.calls != 3 {
		t.Fatal(out, err, store.calls, host.calls)
	}
	first, second := forgeJSON(t, out.Servers[0]), forgeJSON(t, out.Servers[1])
	if first["name"] != "zeta" || second["name"] != "alpha" || first["attached"] != true || first["tool_count"] != float64(0) || second["attached"] != true || second["tool_count"] != float64(-2) {
		t.Fatal(out)
	}
	store.rows = nil
	host.snapshots = []map[string]int{{"orphan": 2}}
	host.calls = 0
	empty, err := NewMCPCatalog(store, host).List(context.Background(), MCPListInput{})
	if err != nil || empty.Servers == nil || empty.Count != 0 || empty.AttachedCount != 1 || host.calls != 1 {
		t.Fatal(empty, err)
	}
	m := forgeJSON(t, empty)
	if len(m) != 3 || !reflect.DeepEqual(m["servers"], []any{}) || m["count"] != float64(0) || m["attached_count"] != float64(1) {
		t.Fatal(m)
	}
}
func TestMCPCatalogTypedViewKeepsExactNumbersAndCollectionOwnership(t *testing.T) {
	srv := mcp.Server{Name: "owned", Args: []string{"argument"}, ToolAllow: []string{"allowed"}, Env: map[string]string{"OWNED": "owned-value"}, Headers: map[string]string{"Owned": "owned-header"}, CreatedMS: 9007199254740993, UpdatedMS: 9223372036854775807}
	host := &mcpAttachmentProbe{snapshots: []map[string]int{{"owned": 0}}}
	view := NewMCPCatalog(nil, host).View(srv)
	if view.CreatedMS != srv.CreatedMS || view.UpdatedMS != srv.UpdatedMS || view.ToolCount == nil || *view.ToolCount != 0 {
		t.Fatal(view)
	}
	raw, _ := json.Marshal(view)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	if string(fields["created_ms"]) != "9007199254740993" || string(fields["updated_ms"]) != "9223372036854775807" {
		t.Fatal(string(raw))
	}
	view.EnvKeys[0] = "changed"
	view.Args[0] = "changed"
	view.ToolAllow[0] = "changed"
	*view.ToolCount = 9
	again := NewMCPCatalog(nil, host).View(srv)
	if !reflect.DeepEqual(again.EnvKeys, []string{"OWNED"}) || srv.Env["OWNED"] != "owned-value" || srv.Headers["Owned"] != "owned-header" || srv.Args[0] != "argument" || srv.ToolAllow[0] != "allowed" || *again.ToolCount != 0 {
		t.Fatal(again)
	}
}
