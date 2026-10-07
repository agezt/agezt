// SPDX-License-Identifier: MIT
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	appcenter "github.com/agezt/agezt/kernel/app/configcenter"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/platform/schema"
)

func centerCLIEndpoint(t *testing.T, command string, value any, failure string) <-chan controlplane.Request {
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
	for name, text := range map[string]string{"control.addr": ln.Addr().String() + "\n", "control.token": "owned-token\n"} {
		if err := os.WriteFile(filepath.Join(runtimeDir, name), []byte(text), 0600); err != nil {
			ln.Close()
			t.Fatal(err)
		}
	}
	ops, err := appcenter.Operations(func(context.Context) *appcenter.Reads { return nil }, func(context.Context) *appcenter.Writes { return nil })
	if err != nil {
		t.Fatal(err)
	}
	var declaration json.RawMessage
	for _, op := range ops {
		if op.Spec().Name == command {
			declaration = op.Spec().OutputSchema
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if failure == "" && command != "" {
		if len(declaration) == 0 {
			t.Fatal("undeclared fixture", command)
		}
		if err := schema.ValidateJSON(declaration, raw); err != nil {
			t.Fatal("fixture violates native schema", err)
		}
	}
	var result map[string]any
	if value != nil {
		if err := json.Unmarshal(raw, &result); err != nil {
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
				if json.NewDecoder(conn).Decode(&req) != nil {
					return
				}
				requests <- req
				reply := controlplane.Response{ID: req.ID, Type: controlplane.RespResult, Result: result}
				if req.Cmd != controlplane.CmdStatus && req.Cmd != command {
					t.Errorf("unexpected native operation: %s want %s", req.Cmd, command)
				}
				if req.Cmd == command && failure != "" {
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
func centerCLIInvoke(args []string) (code int, out, stderr string, panicValue any) {
	var stdout, errOut bytes.Buffer
	defer func() { out = stdout.String(); stderr = errOut.String(); panicValue = recover() }()
	code = cmdConfigCenter(args, &stdout, &errOut)
	return
}
func centerCLIAssertWire(t *testing.T, requests <-chan controlplane.Request, command string, want map[string]any) {
	t.Helper()
	for _, expected := range []string{controlplane.CmdStatus, command} {
		select {
		case req := <-requests:
			if req.Cmd != expected || req.Token != "owned-token" {
				t.Fatal(req)
			}
			if expected == command && !reflect.DeepEqual(req.Args, want) {
				t.Fatal("native args", req.Args, want)
			}
		case <-time.After(time.Second):
			t.Fatal("missing native request", expected)
		}
	}
	if len(requests) != 0 {
		t.Fatal("extra request")
	}
}

func TestCmdConfigCenterNativeSecondsAuditFieldsAndFilterFlags(t *testing.T) {
	const seconds int64 = 1700000000
	entry := appcenter.EntryRow{Key: "owned", Value: "raw", Rating: "public", UpdatedAt: seconds}
	access := appcenter.AccessLogOutput{Count: 1, Logs: []appcenter.AccessLogRow{{Timestamp: seconds, Key: "owned", AgentID: "owned-agent", RunID: "owned-run", Rating: "public", Decision: "denied", Reason: "owned reason", ValueLog: "REDACTED"}}}
	audit := appcenter.AuditOutput{Count: 1, Entries: []appcenter.AuditRow{{Timestamp: seconds, Event: "config.access", Key: "owned", AgentID: "owned-agent", RunID: "owned-run", Rating: "public", Decision: "denied", Reason: "owned reason", Policy: "deny"}}}
	for _, tc := range []struct {
		name     string
		args     []string
		command  string
		value    any
		wire     map[string]any
		contains []string
	}{
		{"get-seconds", []string{"get", "owned"}, controlplane.CmdConfigCenterGet, appcenter.GetOutput{Entry: entry}, map[string]any{"key": "owned"}, []string{time.Unix(seconds, 0).Format(time.RFC3339)}},
		{"access-seconds", []string{"access-log"}, controlplane.CmdConfigCenterAccessLog, access, nil, []string{time.Unix(seconds, 0).Format("06-01-02 15:04:05"), "owned-agent"}},
		{"audit-schema", []string{"audit"}, controlplane.CmdConfigCenterAudit, audit, nil, []string{time.Unix(seconds, 0).Format("06-01-02 15:04:05"), "owned-agent", "decision=denied", "policy=deny", "reason=owned reason"}},
		{"access-filter-flags", []string{"access-log", "--key", " raw ", "--agent", " Agent ", "--since", "-1h", "--json"}, controlplane.CmdConfigCenterAccessLog, access, map[string]any{"key": " raw ", "agent_id": " Agent ", "since": "-1h"}, []string{`"logs"`}},
		{"audit-since", []string{"audit", "--since", "1h", "--json"}, controlplane.CmdConfigCenterAudit, audit, map[string]any{"since": "1h"}, []string{`"entries"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := centerCLIEndpoint(t, tc.command, tc.value, "")
			code, out, stderr, panicValue := centerCLIInvoke(tc.args)
			if panicValue != nil || code != 0 || stderr != "" {
				t.Fatalf("EXPECTED:valid native CLI success ACTUAL:code=%d panic=%v stderr=%q", code, panicValue, stderr)
			}
			for _, part := range tc.contains {
				if !strings.Contains(out, part) {
					t.Errorf("EXPECTED:%s ACTUAL:%s", part, out)
				}
			}
			centerCLIAssertWire(t, requests, tc.command, tc.wire)
		})
	}
}

func TestCmdConfigCenterMissingFlagValuesRejectBeforeDial(t *testing.T) {
	for _, args := range [][]string{{"access-log", "--key"}, {"access-log", "--agent"}, {"access-log", "--since"}, {"audit", "--since"}, {"list", "--rating"}, {"rating", "owned", "--rating"}} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			requests := centerCLIEndpoint(t, "", nil, "")
			code, _, stderr, panicValue := centerCLIInvoke(args)
			if code != 2 || panicValue != nil || !strings.Contains(stderr, "requires a value") || len(requests) != 0 {
				t.Errorf("EXPECTED:missing flag value rejected before dial ACTUAL:code=%d panic=%v stderr=%q requests=%d", code, panicValue, stderr, len(requests))
			}
		})
	}
}

func TestCmdConfigCenterNativeWireRenderingEmptyAndErrorBranches(t *testing.T) {
	const seconds int64 = 1700000000
	secret := "owned-sensitive-middle-value"
	public := appcenter.EntryRow{Key: "owned", Value: " raw ", Rating: "public", UpdatedAt: seconds, Description: " raw description ", AllowedAgents: []string{"First", "Second"}, ExcludedAgents: []string{"Denied"}}
	masked := appcenter.EntryRow{Key: "owned", Value: creds.MaskValue(secret), Rating: "secret", UpdatedAt: seconds, Masked: true}
	list := appcenter.ListOutput{Count: 4, Entries: []appcenter.EntryRow{{Key: "public-b", Value: "b", Rating: "public"}, {Key: "public-a", Value: "a", Rating: "public"}, {Key: "restricted", Value: "r", Rating: "restricted"}, masked}}
	stats := map[string]any{"total_entries": 0, "by_rating": map[string]int{}}
	for _, tc := range []struct {
		name     string
		args     []string
		command  string
		value    any
		wire     map[string]any
		code     int
		contains []string
		failure  string
	}{
		{"set-default", []string{"set", " raw key ", " raw value "}, controlplane.CmdConfigCenterSet, appcenter.SetOutput{Entry: public}, map[string]any{"key": " raw key ", "value": " raw value ", "rating": "internal"}, 0, []string{"rating: public"}, ""},
		{"set-flags", []string{"set", "owned", "value", "--rating", "PUBLIC", "--description", " raw ", "--allow-agents", "First,Second", "--deny-agents", "Denied"}, controlplane.CmdConfigCenterSet, appcenter.SetOutput{Entry: public}, map[string]any{"key": "owned", "value": "value", "rating": "public", "description": " raw ", "allowed_agents": []any{"First", "Second"}, "excluded_agents": []any{"Denied"}}, 0, []string{"description:  raw description ", "allow agents: First, Second", "deny agents: Denied"}, ""},
		{"set-secret", []string{"set", "owned", secret, "--rating", "secret"}, controlplane.CmdConfigCenterSet, appcenter.SetOutput{Entry: masked}, map[string]any{"key": "owned", "value": secret, "rating": "secret"}, 0, []string{creds.MaskValue(secret)}, ""},
		{"get-secret", []string{"get", "owned"}, controlplane.CmdConfigCenterGet, appcenter.GetOutput{Entry: masked}, map[string]any{"key": "owned"}, 0, []string{creds.MaskValue(secret), "rating:  secret"}, ""},
		{"list-text", []string{"list"}, controlplane.CmdConfigCenterList, list, nil, 0, []string{"4 entries", "public-a", "public-b", "********"}, ""},
		{"list-json", []string{"list", "--rating", "secret", "--json"}, controlplane.CmdConfigCenterList, list, map[string]any{"rating": "secret"}, 0, []string{`"masked": true`}, ""},
		{"list-positional", []string{"list", "public", "--json"}, controlplane.CmdConfigCenterList, appcenter.ListOutput{Entries: []appcenter.EntryRow{}}, map[string]any{"rating": "public"}, 0, []string{`"entries": []`}, ""},
		{"list-empty", []string{"list"}, controlplane.CmdConfigCenterList, appcenter.ListOutput{Entries: []appcenter.EntryRow{}}, nil, 0, []string{"0 entries"}, ""},
		{"delete", []string{"delete", " raw "}, controlplane.CmdConfigCenterDelete, appcenter.DeleteOutput{Deleted: true}, map[string]any{"key": " raw "}, 0, []string{" raw  deleted"}, ""},
		{"delete-false", []string{"delete", "owned"}, controlplane.CmdConfigCenterDelete, appcenter.DeleteOutput{}, map[string]any{"key": "owned"}, 0, []string{"owned not found"}, ""},
		{"rating-read", []string{"rating", "owned"}, controlplane.CmdConfigCenterGet, appcenter.GetOutput{Entry: public}, map[string]any{"key": "owned"}, 0, []string{"owned: public"}, ""},
		{"rating-set", []string{"rating", "owned", "--rating", "secret"}, controlplane.CmdConfigCenterSetRating, appcenter.SetRatingOutput{Override: true}, map[string]any{"key": "owned", "rating": "secret"}, 0, []string{"rating set to secret", "manual override"}, ""},
		{"rating-positional", []string{"rating", "owned", "public"}, controlplane.CmdConfigCenterSetRating, appcenter.SetRatingOutput{}, map[string]any{"key": "owned", "rating": "public"}, 0, []string{"rating set to public"}, ""},
		{"access-empty", []string{"access-log"}, controlplane.CmdConfigCenterAccessLog, appcenter.AccessLogOutput{Logs: []appcenter.AccessLogRow{}}, nil, 0, []string{"No access logs found"}, ""},
		{"audit-empty", []string{"audit"}, controlplane.CmdConfigCenterAudit, appcenter.AuditOutput{Entries: []appcenter.AuditRow{}}, nil, 0, []string{"No audit entries found"}, ""},
		{"health", []string{"health"}, controlplane.CmdConfigCenterHealth, appcenter.HealthOutput{Status: "healthy", Checks: map[string]string{"store": "ok", "config_center": "ok"}, Stats: &stats}, nil, 0, []string{"Status: healthy", "store: ok", "total_entries: 0"}, ""},
		{"health-unavailable", []string{"health", "ignored"}, controlplane.CmdConfigCenterHealth, appcenter.HealthOutput{Status: "unavailable", Checks: map[string]string{"config_center": "not configured"}}, nil, 0, []string{"Status: unavailable", "not configured"}, ""},
		{"set-error", []string{"set", "owned", "value"}, controlplane.CmdConfigCenterSet, nil, map[string]any{"key": "owned", "value": "value", "rating": "internal"}, 1, []string{"owned failure"}, "owned failure"},
		{"get-error", []string{"get", "owned"}, controlplane.CmdConfigCenterGet, nil, map[string]any{"key": "owned"}, 1, []string{"owned failure"}, "owned failure"},
		{"list-error", []string{"list"}, controlplane.CmdConfigCenterList, nil, nil, 1, []string{"owned failure"}, "owned failure"},
		{"delete-error", []string{"delete", "owned"}, controlplane.CmdConfigCenterDelete, nil, map[string]any{"key": "owned"}, 1, []string{"owned failure"}, "owned failure"},
		{"rating-error", []string{"rating", "owned", "public"}, controlplane.CmdConfigCenterSetRating, nil, map[string]any{"key": "owned", "rating": "public"}, 1, []string{"owned failure"}, "owned failure"},
		{"access-error", []string{"access-log"}, controlplane.CmdConfigCenterAccessLog, nil, nil, 1, []string{"owned failure"}, "owned failure"},
		{"audit-error", []string{"audit"}, controlplane.CmdConfigCenterAudit, nil, nil, 1, []string{"owned failure"}, "owned failure"},
		{"health-error", []string{"health"}, controlplane.CmdConfigCenterHealth, nil, nil, 1, []string{"owned failure"}, "owned failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := centerCLIEndpoint(t, tc.command, tc.value, tc.failure)
			code, out, stderr, panicValue := centerCLIInvoke(tc.args)
			if code != tc.code || panicValue != nil {
				t.Fatal(code, panicValue, out, stderr)
			}
			for _, part := range tc.contains {
				if !strings.Contains(out+stderr, part) {
					t.Fatal(part, out, stderr)
				}
			}
			if strings.Contains(out, "sensitive-middle") || tc.failure != "" && out != "" {
				t.Fatal("private/error output", out)
			}
			if tc.name == "list-text" && (strings.Index(out, "public-a") > strings.Index(out, "public-b") || strings.Index(out, "restricted") > strings.Index(out, "public-a")) {
				t.Fatal("list order", out)
			}
			if tc.name == "rating-positional" && strings.Contains(out, "manual override") {
				t.Fatal(out)
			}
			centerCLIAssertWire(t, requests, tc.command, tc.wire)
		})
	}
}

func TestCmdConfigCenterInvalidAndHelpDoNotDial(t *testing.T) {
	requests := centerCLIEndpoint(t, "", nil, "")
	for _, args := range [][]string{nil, {"unknown"}, {"set"}, {"set", "owned"}, {"set", "owned", "value", "--rating", "invalid"}, {"set", "owned", "value", "--description"}, {"get"}, {"get", "owned", "extra"}, {"delete"}, {"delete", "owned", "extra"}, {"rating"}, {"rating", "owned", "PUBLIC"}, {"rating", "owned", "public", "extra"}, {"list", "--unknown"}, {"list", "public", "extra"}, {"access-log", "--unknown"}, {"access-log", "extra"}, {"access-log", "--key", "--json"}, {"access-log", "--since", "--agent"}, {"audit", "--unknown"}, {"audit", "extra"}, {"audit", "--since", "--json"}, {"list", "--rating", "--json"}} {
		code, _, stderr, panicValue := centerCLIInvoke(args)
		if code != 2 || panicValue != nil || stderr == "" || len(requests) != 0 {
			t.Fatal(args, code, stderr, panicValue, len(requests))
		}
	}
	for _, sub := range []string{"", "set", "get", "list", "delete", "rating", "access-log", "audit", "health"} {
		args := []string{"--help"}
		if sub != "" {
			args = append([]string{sub}, args...)
		}
		code, out, stderr, panicValue := centerCLIInvoke(args)
		if code != 0 || out == "" || stderr != "" || panicValue != nil || len(requests) != 0 {
			t.Fatal(args, code, panicValue)
		}
	}
}
