// SPDX-License-Identifier: MIT
package main

import (
	"bytes"
	"encoding/json"
	"github.com/agezt/agezt/kernel/controlplane"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// This endpoint only records native requests and returns canned replies. It is
// neither a daemon nor an MCP peer, and it cannot start tools or mutate a store.
func mcpCLIEndpoint(t *testing.T) <-chan controlplane.Request {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AGEZT_HOME", dir)
	t.Setenv("AGEZT_TOKEN", "")
	path := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	for file, value := range map[string]string{"control.addr": ln.Addr().String() + "\n", "control.token": "owned-token\n"} {
		if err := os.WriteFile(filepath.Join(path, file), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	requests := make(chan controlplane.Request, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			func() {
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(3 * time.Second))
				var req controlplane.Request
				if err := json.NewDecoder(conn).Decode(&req); err != nil {
					return
				}
				requests <- req
				result := map[string]any{}
				switch req.Cmd {
				case controlplane.CmdMCPList:
					result = map[string]any{"servers": []any{map[string]any{"name": "owned", "command": "never-executed", "args": []any{" raw "}, "attached": true, "tool_count": 0, "enabled": true, "lazy": true}}, "count": 1, "attached_count": 1}
				case controlplane.CmdMCPAttach:
					result = map[string]any{"tools": []any{"mcp_owned_second", "mcp_owned_first"}}
				case controlplane.CmdMCPRemove:
					result = map[string]any{"removed": req.Args["ref"] != "missing"}
				}
				json.NewEncoder(conn).Encode(controlplane.Response{ID: req.ID, Type: controlplane.RespResult, Result: result})
			}()
		}
	}()
	t.Cleanup(func() { ln.Close(); <-done })
	return requests
}
func TestCmdMCPNativeRequestsAndResponses(t *testing.T) {
	requests := mcpCLIEndpoint(t)
	cases := []struct {
		args     []string
		cmd      string
		wire     map[string]any
		code     int
		contains string
	}{
		{[]string{"list"}, controlplane.CmdMCPList, nil, 0, "ATTACHED (0 tools)"},
		{[]string{"list", "--json"}, controlplane.CmdMCPList, nil, 0, `"attached_count": 1`},
		{[]string{"add", "owned", "--cmd", "never-executed", "--arg", " raw ", "--desc", " raw description ", "--lazy"}, controlplane.CmdMCPAdd, map[string]any{"server": map[string]any{"name": "owned", "command": "never-executed", "args": []any{" raw "}, "description": " raw description ", "lazy": true}}, 0, "registered owned"},
		{[]string{"register", "remote", "--cmd", "ignored", "--url", "http://127.0.0.1:1/owned", "--header", "X-Owned: owned value", "--arg", " kept "}, controlplane.CmdMCPAdd, map[string]any{"server": map[string]any{"name": "remote", "url": "http://127.0.0.1:1/owned", "headers": map[string]any{"X-Owned": "owned value"}, "args": []any{" kept "}, "description": ""}}, 0, "registered remote"},
		{[]string{"attach", " raw-ref "}, controlplane.CmdMCPAttach, map[string]any{"ref": " raw-ref "}, 0, "mcp_owned_second"},
		{[]string{"detach", " raw-ref "}, controlplane.CmdMCPDetach, map[string]any{"ref": " raw-ref "}, 0, "detached  raw-ref "},
		{[]string{"enable", " raw-ref "}, controlplane.CmdMCPSetEnabled, map[string]any{"ref": " raw-ref ", "enabled": true}, 0, "will auto-attach"},
		{[]string{"disable", " raw-ref "}, controlplane.CmdMCPSetEnabled, map[string]any{"ref": " raw-ref ", "enabled": false}, 0, "will NOT auto-attach"},
		{[]string{"remove", " raw-ref "}, controlplane.CmdMCPRemove, map[string]any{"ref": " raw-ref "}, 0, "removed  raw-ref "},
		{[]string{"rm", "missing"}, controlplane.CmdMCPRemove, map[string]any{"ref": "missing"}, 1, "unknown server"},
	}
	for _, tc := range cases {
		var out, errOut bytes.Buffer
		code := cmdMCP(tc.args, &out, &errOut)
		if code != tc.code || !strings.Contains(out.String()+errOut.String(), tc.contains) {
			t.Fatal(tc.args, code, out.String(), errOut.String())
		}
		for _, expected := range []string{controlplane.CmdStatus, tc.cmd} {
			select {
			case req := <-requests:
				if req.Cmd != expected || req.Token != "owned-token" || (expected == tc.cmd && !reflect.DeepEqual(req.Args, tc.wire)) {
					t.Fatal(tc.args, req, tc.wire)
				}
			case <-time.After(time.Second):
				t.Fatal("native request not recorded", tc.args)
			}
		}
	}
	if len(requests) != 0 {
		t.Fatal("extra request", len(requests))
	}
}
func TestCmdMCPMalformedArgumentsRejectBeforeDial(t *testing.T) {
	requests := mcpCLIEndpoint(t)
	for _, args := range [][]string{{"unknown"}, {"add"}, {"register"}, {"add", "owned"}, {"add", "owned", "--cmd"}, {"add", "owned", "--url"}, {"add", "owned", "--arg"}, {"add", "owned", "--header"}, {"add", "owned", "--desc"}, {"add", "owned", "--url", "owned", "--header", "invalid"}, {"attach"}, {"detach"}, {"remove"}, {"rm"}, {"enable"}, {"disable"}} {
		var out, errOut bytes.Buffer
		code := cmdMCP(args, &out, &errOut)
		if len(requests) != 0 || out.Len() != 0 || errOut.Len() == 0 || (!reflect.DeepEqual(args, []string{"unknown"}) && code != 2) {
			t.Fatal(args, code, out.String(), errOut.String(), len(requests))
		}
	}
}
