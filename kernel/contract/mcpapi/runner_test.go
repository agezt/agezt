// SPDX-License-Identifier: MIT

package mcpapi

import (
	"context"
	"reflect"
	"testing"
)

// TestTheMCPRunnerPortHasExactlySevenMethods: the MCP port is the whole of
// what the MCP tool may do to the daemon's registry — add, attach, detach,
// remove, list, look up, and report what is attached. An eighth method would
// be a new reach into the daemon, and it would have to be earned here.
func TestTheMCPRunnerPortHasExactlySevenMethods(t *testing.T) {
	typ := reflect.TypeOf((*Runner)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Runner must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 7 {
		t.Fatalf("the port has %d methods, want 7", typ.NumMethod())
	}
	for _, name := range []string{
		"AddMCPServer", "AttachMCPServer", "DetachMCPServer",
		"RemoveMCPServer", "MCPServers", "MCPServer", "MCPAttached",
	} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the port must have a %s method", name)
		}
	}
}

// TestTheMCPRunnerNamesOnlyContractRows: every type the port names must be
// the contract's row or a builtin. A module's type in a port's signature
// would drag that module into every plugin that holds the port — the very
// leak the contract layer exists to stop.
func TestTheMCPRunnerNamesOnlyContractRows(t *testing.T) {
	typ := reflect.TypeOf((*Runner)(nil)).Elem()
	server := reflect.TypeOf(Server{})
	errType := reflect.TypeOf((*error)(nil)).Elem()
	str := reflect.TypeOf("")
	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()

	add, _ := typ.MethodByName("AddMCPServer")
	// AddMCPServer(corr string, srv Server) (Server, error)
	if add.Type.NumIn() != 2 || add.Type.In(0) != str || add.Type.In(1) != server {
		t.Errorf("AddMCPServer must take (string, Server), got %v", add.Type)
	}
	if add.Type.NumOut() != 2 || add.Type.Out(0) != server || add.Type.Out(1) != errType {
		t.Errorf("AddMCPServer must answer (Server, error), got %v", add.Type)
	}

	attach, _ := typ.MethodByName("AttachMCPServer")
	// AttachMCPServer(ctx, corr, ref string) (Server, []string, error)
	if attach.Type.NumIn() != 3 || attach.Type.In(0) != ctxType || attach.Type.In(1) != str || attach.Type.In(2) != str {
		t.Errorf("AttachMCPServer must take (ctx, string, string), got %v", attach.Type)
	}
	if attach.Type.NumOut() != 3 || attach.Type.Out(0) != server || attach.Type.Out(1) != reflect.TypeOf([]string{}) || attach.Type.Out(2) != errType {
		t.Errorf("AttachMCPServer must answer (Server, []string, error), got %v", attach.Type)
	}

	remove, _ := typ.MethodByName("RemoveMCPServer")
	// RemoveMCPServer(corr, ref string) (bool, error)
	if remove.Type.NumIn() != 2 || remove.Type.NumOut() != 2 || remove.Type.Out(0) != reflect.TypeOf(false) || remove.Type.Out(1) != errType {
		t.Errorf("RemoveMCPServer must answer (bool, error), got %v", remove.Type)
	}

	list, _ := typ.MethodByName("MCPServers")
	// MCPServers() []Server
	if list.Type.NumIn() != 0 || list.Type.NumOut() != 1 || list.Type.Out(0) != reflect.TypeOf([]Server{}) {
		t.Errorf("MCPServers must answer []Server, got %v", list.Type)
	}

	one, _ := typ.MethodByName("MCPServer")
	// MCPServer(ref string) (Server, bool)
	if one.Type.NumIn() != 1 || one.Type.In(0) != str || one.Type.NumOut() != 2 || one.Type.Out(0) != server || one.Type.Out(1) != reflect.TypeOf(false) {
		t.Errorf("MCPServer must answer (Server, bool), got %v", one.Type)
	}

	attached, _ := typ.MethodByName("MCPAttached")
	// MCPAttached() map[string]int
	if attached.Type.NumIn() != 0 || attached.Type.NumOut() != 1 || attached.Type.Out(0) != reflect.TypeOf(map[string]int{}) {
		t.Errorf("MCPAttached must answer map[string]int, got %v", attached.Type)
	}
}
