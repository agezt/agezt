// SPDX-License-Identifier: MIT
package main

import (
	"bytes"
	"context"
	"encoding/json"
	appconfig "github.com/agezt/agezt/kernel/app/config"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/platform/schema"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The owned endpoint returns only canned config; no daemon or process starts.
func configShowCLIEndpoint(t *testing.T, result map[string]any, failure string) <-chan controlplane.Request {
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
func TestCmdConfigShowNativeWireRenderingAndPrivacy(t *testing.T) {
	rich := map[string]any{"paths": map[string]any{"base": " raw base ", "journal": "owned-journal", "state": "owned-state", "runtime": "owned-runtime", "catalog": "owned-catalog", "vault": "owned-vault"}, "model": " raw model ", "system_prompt_set": true, "tool_count": -2, "plugin_count": 0, "ask_policy": "ask", "env": map[string]any{"AGEZT_MODEL": true, "AGEZT_DISCORD_TOKEN": true}, "routing": map[string]any{"routes": map[string]any{"z": []any{"second", "first"}, "a": []any{}}, "requires": map[string]any{"a": []any{" raw cap "}}, "model_overrides": map[string]any{"a": " raw override ", "empty": ""}}}
	zero := map[string]any{"paths": map[string]any{"base": "", "journal": "", "state": "", "runtime": "", "catalog": "", "vault": ""}, "model": "", "system_prompt_set": false, "tool_count": 0, "plugin_count": 0, "ask_policy": "allow", "env": map[string]any{}}
	for _, tc := range []struct {
		name     string
		args     []string
		result   map[string]any
		failure  string
		code     int
		contains []string
	}{
		{"table", []string{"show"}, rich, "", 0, []string{" raw base ", " raw model ", "set (content not shown)", "tools           : -2 registered", "plugins         : 0 spawned", "routing (effective)", "[second first]", "[ raw cap ]", " raw override ", "2 set (values not shown)", "AGEZT_DISCORD_TOKEN\n    AGEZT_MODEL"}},
		{"json", []string{"show", "--json"}, rich, "", 0, []string{`"system_prompt_set": true`, `"tool_count": -2`, `"plugin_count": 0`, `"AGEZT_DISCORD_TOKEN": true`, `"a": []`, `"empty": ""`}},
		{"default", []string{"show"}, zero, "", 0, []string{"(provider default)", "system prompt   : unset", "env (AGEZT_*)   : none set"}},
		{"empty-json", []string{"show", "--json"}, zero, "", 0, []string{`"env": {}`, `"model": ""`, `"system_prompt_set": false`}},
		{"error", []string{"show"}, nil, "owned error", 1, []string{"owned error"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.failure == "" {
				ops, err := appconfig.Operations(func(context.Context) *appconfig.Service { return nil })
				if err != nil {
					t.Fatal(err)
				}
				raw, err := json.Marshal(tc.result)
				if err != nil {
					t.Fatal(err)
				}
				if err := schema.ValidateJSON(ops[0].Spec().OutputSchema, raw); err != nil {
					t.Fatal("CLI fixture violates typed native schema", err)
				}
			}
			requests := configShowCLIEndpoint(t, tc.result, tc.failure)
			var out, errOut bytes.Buffer
			code := cmdConfig(tc.args, &out, &errOut)
			if code != tc.code {
				t.Fatal(code, out.String(), errOut.String())
			}
			for _, part := range tc.contains {
				if !strings.Contains(out.String()+errOut.String(), part) {
					t.Fatal(part, out.String(), errOut.String())
				}
			}
			for _, expected := range []string{controlplane.CmdStatus, controlplane.CmdConfig} {
				select {
				case req := <-requests:
					if req.Cmd != expected || req.Token != "owned-token" || req.Args != nil {
						t.Fatal(req)
					}
				case <-time.After(time.Second):
					t.Fatal("missing request", expected)
				}
			}
			if len(requests) != 0 {
				t.Fatal("extra requests")
			}
			if tc.failure != "" && out.Len() != 0 {
				t.Fatal("error emitted success", out.String())
			}
			if tc.name == "table" {
				text := out.String()
				last := -1
				for _, value := range []string{" raw base ", "owned-journal", "owned-state", "owned-runtime", "owned-catalog", "owned-vault"} {
					index := strings.Index(text, value)
					if index <= last {
						t.Fatal("path order", text)
					}
					last = index
				}
				if strings.Contains(text, "empty        ") {
					t.Fatal("empty model override printed", text)
				}
			}
			if tc.name == "default" && strings.Contains(out.String(), "routing (effective)") {
				t.Fatal(out.String())
			}
		})
	}
}
func TestCmdConfigShowNativeInvalidHelpDoNotDial(t *testing.T) {
	requests := configShowCLIEndpoint(t, nil, "")
	for _, args := range [][]string{{"show", "unexpected"}, {"show", "--json", "unexpected"}, {"show", "--help"}, {"show", "-h"}} {
		var out, errOut bytes.Buffer
		expected := 2
		if args[len(args)-1] == "--help" || args[len(args)-1] == "-h" {
			expected = 0
		}
		if code := cmdConfig(args, &out, &errOut); code != expected {
			t.Fatal(args, code, out.String(), errOut.String())
		}
	}
	select {
	case req := <-requests:
		t.Fatal("validation dialed", req)
	default:
	}
}
