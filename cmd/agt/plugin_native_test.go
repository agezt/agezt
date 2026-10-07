// SPDX-License-Identifier: MIT
package main

import (
	"bytes"
	"encoding/json"
	"github.com/agezt/agezt/kernel/controlplane"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The owned endpoint returns only canned inventory; no daemon or process starts.
func pluginInventoryCLIEndpoint(t *testing.T, result map[string]any, failure string) <-chan controlplane.Request {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AGEZT_HOME", dir)
	t.Setenv("AGEZT_TOKEN", "")
	runtimeDir := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	for file, value := range map[string]string{"control.addr": ln.Addr().String() + "\n", "control.token": "owned-token\n"} {
		if err := os.WriteFile(filepath.Join(runtimeDir, file), []byte(value), 0600); err != nil {
			ln.Close()
			t.Fatal(err)
		}
	}
	requests := make(chan controlplane.Request, 8)
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
				if json.NewDecoder(conn).Decode(&req) != nil {
					return
				}
				requests <- req
				reply := controlplane.Response{ID: req.ID, Type: controlplane.RespResult, Result: result}
				if failure != "" {
					reply.Type = controlplane.RespError
					reply.Error = failure
					reply.Result = nil
				}
				json.NewEncoder(conn).Encode(reply)
			}()
		}
	}()
	t.Cleanup(func() { ln.Close(); <-done })
	return requests
}
func TestCmdPluginInventoryNativeWireAndRendering(t *testing.T) {
	rows := []any{map[string]any{"prefix": "a", "path": " raw path ", "args": []any{" raw arg ", "second"}, "tool_count": -2, "hash_pinned": true, "allowed_tools": nil}, map[string]any{"prefix": "b", "path": "never-executed", "args": []any{}, "tool_count": 0, "hash_pinned": false, "allowed_tools": []any{}}, map[string]any{"prefix": "c", "path": "never-executed", "args": []any{}, "tool_count": 1, "hash_pinned": false, "allowed_tools": []any{" second ", "first"}}}
	for _, tc := range []struct {
		name     string
		args     []string
		result   map[string]any
		failure  string
		code     int
		contains []string
	}{
		{"table", []string{"list"}, map[string]any{"plugins": rows, "count": 3}, "", 0, []string{"3 plugin(s):", "a [pinned]", "path  :  raw path ", "args  :  raw arg  second", "tools : -2 registered", "allow : unrestricted", "allow : \n", "allow :  second , first"}},
		{"json", []string{"list", "--json"}, map[string]any{"plugins": rows, "count": 3}, "", 0, []string{`"allowed_tools": null`, `"allowed_tools": []`, `"tool_count": -2`, `"hash_pinned": false`}},
		{"empty", []string{"list"}, map[string]any{"plugins": []any{}, "count": 0}, "", 0, []string{"no external plugins loaded"}},
		{"error", []string{"list"}, nil, "owned failure", 1, []string{"owned failure"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := pluginInventoryCLIEndpoint(t, tc.result, tc.failure)
			var out, errOut bytes.Buffer
			code := cmdPlugin(tc.args, &out, &errOut)
			if code != tc.code {
				t.Fatal(code, out.String(), errOut.String())
			}
			for _, part := range tc.contains {
				if !strings.Contains(out.String()+errOut.String(), part) {
					t.Fatal(part, out.String(), errOut.String())
				}
			}
			for _, expected := range []string{controlplane.CmdStatus, controlplane.CmdPluginList} {
				select {
				case req := <-requests:
					if req.Cmd != expected || req.Token != "owned-token" || req.Args != nil {
						t.Fatal(req)
					}
				case <-time.After(time.Second):
					t.Fatal("no native request", expected)
				}
			}
			if len(requests) != 0 {
				t.Fatal("extra request", len(requests))
			}
			if tc.failure != "" && out.Len() != 0 {
				t.Fatal(out.String())
			}
		})
	}
}
func TestCmdPluginInventoryInvalidAndHelpDoNotDial(t *testing.T) {
	requests := pluginInventoryCLIEndpoint(t, nil, "")
	for _, args := range [][]string{{"list", "unexpected"}, {"list", "--json", "unexpected"}, {"list", "--help"}, {"list", "-h"}} {
		var out, errOut bytes.Buffer
		code := cmdPlugin(args, &out, &errOut)
		expected := 2
		if args[len(args)-1] == "--help" || args[len(args)-1] == "-h" {
			expected = 0
		}
		if code != expected {
			t.Fatal(args, code, out.String(), errOut.String())
		}
	}
	select {
	case req := <-requests:
		t.Fatal("validation dialed", req)
	default:
	}
}
