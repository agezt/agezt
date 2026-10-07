// SPDX-License-Identifier: MIT
package main

import (
	"bytes"
	"encoding/json"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
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
func marketCLIEndpoint(t *testing.T) <-chan controlplane.Request {
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
				case controlplane.CmdMarketList:
					result = map[string]any{"packs": []any{map[string]any{"name": "owned", "category": "owned-category", "description": "owned description", "installed": true}}, "count": 1}
				case controlplane.CmdMarketShow:
					result = map[string]any{"pack": map[string]any{"name": "owned", "version": "1.0.0", "category": "owned-category"}, "skill_count": 1, "mcp_count": 0, "tool_count": 1, "installed": false, "vet": map[string]any{"verdict": "clean"}}
				case controlplane.CmdMarketInstall:
					json.NewEncoder(conn).Encode(controlplane.Response{ID: req.ID, Type: controlplane.RespEvent, Event: &event.Event{Kind: event.KindMarketInstallProgress, Subject: "market.install", Actor: "market", Payload: json.RawMessage(`{"stage":"owned","ok":true}`)}})
					result = map[string]any{"name": "owned", "version": "1.0.0", "skill_ids": []any{"skill"}, "mcp_servers": []any{}, "tool_reqs": []any{"owned-tool"}, "unsigned": true}
				case controlplane.CmdMarketUninstall:
					json.NewEncoder(conn).Encode(controlplane.Response{ID: req.ID, Type: controlplane.RespEvent, Event: &event.Event{Kind: event.KindMarketUninstallProgress, Subject: "market.uninstall", Actor: "market", Payload: json.RawMessage(`{"stage":"owned","ok":true}`)}})
					result = map[string]any{"uninstalled": "owned"}
				case controlplane.CmdMarketSources:
					result = map[string]any{"sources": []any{map[string]any{"name": "owned", "url": "http://owned/index.json", "pubkey": "owned-public-key"}}, "count": 1}
				case controlplane.CmdMarketAddSource:
					result = map[string]any{"name": "reported", "url": "http://owned/index.json"}
				case controlplane.CmdMarketRemoveSource:
					result = map[string]any{"name": req.Args["name"], "removed": req.Args["name"] != "missing"}
				case controlplane.CmdMarketSync:
					result = map[string]any{"results": []any{map[string]any{"source": "owned", "packs": 2}}, "synced": 1, "packs": 2, "partial_error": "owned partial error"}
				}
				if req.Args["name"] == "fail" && (req.Cmd == controlplane.CmdMarketInstall || req.Cmd == controlplane.CmdMarketUninstall) {
					json.NewEncoder(conn).Encode(controlplane.Response{ID: req.ID, Type: controlplane.RespError, Error: "owned terminal failure after progress"})
				} else {
					json.NewEncoder(conn).Encode(controlplane.Response{ID: req.ID, Type: controlplane.RespResult, Result: result})
				}
			}()
		}
	}()
	t.Cleanup(func() { ln.Close(); <-done })
	return requests
}

func TestCmdMarketNativeRequestsAndStreamingResults(t *testing.T) {
	requests := marketCLIEndpoint(t)
	cases := []struct {
		args     []string
		cmd      string
		wire     map[string]any
		contains string
	}{
		{[]string{"list"}, controlplane.CmdMarketList, map[string]any{"query": ""}, "1 pack(s)"},
		{[]string{"ls", "--json"}, controlplane.CmdMarketList, map[string]any{"query": ""}, `"count": 1`},
		{[]string{"search", " raw query "}, controlplane.CmdMarketList, map[string]any{"query": " raw query "}, "owned description"},
		{[]string{"show", " owned ", "--marketplace", " raw-market "}, controlplane.CmdMarketShow, map[string]any{"name": " owned ", "marketplace": " raw-market "}, "security review: clean"},
		{[]string{"get", "owned", "--json"}, controlplane.CmdMarketShow, map[string]any{"name": "owned", "marketplace": ""}, `"skill_count": 1`},
		{[]string{"install", " owned ", "--marketplace", " raw-market ", "--version", " raw-version "}, controlplane.CmdMarketInstall, map[string]any{"name": " owned ", "marketplace": " raw-market ", "version": " raw-version "}, "owned-tool"},
		{[]string{"install", "owned", "--json"}, controlplane.CmdMarketInstall, map[string]any{"name": "owned", "marketplace": "", "version": ""}, `"unsigned": true`},
		{[]string{"uninstall", " owned "}, controlplane.CmdMarketUninstall, map[string]any{"name": " owned "}, "uninstalled  owned "},
		{[]string{"uninstall", "owned", "--json"}, controlplane.CmdMarketUninstall, map[string]any{"name": "owned"}, `"uninstalled": "owned"`},
		{[]string{"sources"}, controlplane.CmdMarketSources, nil, "signed-key pinned"},
		{[]string{"sources", "--json"}, controlplane.CmdMarketSources, nil, `"count": 1`},
		{[]string{"add", " http://owned/index.json ", "--name", " raw-name ", "--pubkey", " raw-key "}, controlplane.CmdMarketAddSource, map[string]any{"url": " http://owned/index.json ", "name": " raw-name ", "pubkey": " raw-key "}, "reported"},
		{[]string{"remove", " owned "}, controlplane.CmdMarketRemoveSource, map[string]any{"name": " owned "}, "removed source"},
		{[]string{"rm", "missing"}, controlplane.CmdMarketRemoveSource, map[string]any{"name": "missing"}, "no source named"},
		{[]string{"sync"}, controlplane.CmdMarketSync, map[string]any{"name": ""}, "warning: some sources failed: owned partial error"},
		{[]string{"sync", " raw-source ", "--json"}, controlplane.CmdMarketSync, map[string]any{"name": " raw-source "}, `"synced": 1`},
	}
	for _, tc := range cases {
		var out, errOut bytes.Buffer
		code := cmdMarket(tc.args, &out, &errOut)
		if code != 0 || !strings.Contains(out.String()+errOut.String(), tc.contains) {
			t.Fatal(tc.args, code, out.String(), errOut.String())
		}
		for _, expected := range []string{controlplane.CmdStatus, tc.cmd} {
			select {
			case req := <-requests:
				if req.Cmd != expected || req.Token != "owned-token" || expected == tc.cmd && !reflect.DeepEqual(req.Args, tc.wire) {
					t.Fatal(tc.args, req, tc.wire)
				}
			case <-time.After(time.Second):
				t.Fatal("missing request", tc.args)
			}
		}
	}
	if len(requests) != 0 {
		t.Fatal("extra requests", len(requests))
	}
}
func TestCmdMarketRejectedInputsDoNotDial(t *testing.T) {
	requests := marketCLIEndpoint(t)
	for _, args := range [][]string{nil, {"unknown"}, {"search"}, {"list", "unexpected"}, {"sources", "unexpected"}, {"show"}, {"install"}, {"uninstall"}, {"add"}, {"remove"}, {"show", "owned", "extra"}, {"install", "owned", "extra"}, {"uninstall", "owned", "extra"}, {"add", "owned", "extra"}, {"remove", "owned", "extra"}, {"sync", "owned", "extra"}} {
		var out, errOut bytes.Buffer
		if code := cmdMarket(args, &out, &errOut); code != 2 || out.Len() != 0 || errOut.Len() == 0 || len(requests) != 0 {
			t.Fatal(args, code, out.String(), errOut.String(), len(requests))
		}
	}
}

func TestCmdMarketStreamTerminalErrorsAfterProgress(t *testing.T) {
	requests := marketCLIEndpoint(t)
	for _, verb := range []string{"install", "uninstall"} {
		var out, errOut bytes.Buffer
		if code := cmdMarket([]string{verb, "fail"}, &out, &errOut); code != 1 || out.Len() != 0 || !strings.Contains(errOut.String(), "owned terminal failure after progress") {
			t.Fatal(verb, code, out.String(), errOut.String())
		}
		for i := 0; i < 2; i++ {
			select {
			case <-requests:
			case <-time.After(time.Second):
				t.Fatal("missing request")
			}
		}
	}
	if len(requests) != 0 {
		t.Fatal("extra requests")
	}
}
