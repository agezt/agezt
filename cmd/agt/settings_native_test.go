// SPDX-License-Identifier: MIT
package main

import (
	"bytes"
	"context"
	"encoding/json"
	appsettings "github.com/agezt/agezt/kernel/app/settings"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/settings"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type settingsCLIReply struct {
	fail    string
	pinned  bool
	applied string
	removed bool
}

func settingsCLIEndpoint(t *testing.T, reply settingsCLIReply) <-chan controlplane.Request {
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
	ops, err := appsettings.Operations(func(context.Context) *appsettings.Reads { return nil }, func(context.Context) *appsettings.Writes { return nil })
	if err != nil {
		t.Fatal(err)
	}
	schemas := map[string]json.RawMessage{}
	for _, op := range ops {
		schemas[op.Spec().Name] = op.Spec().OutputSchema
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
				var value any = map[string]any{}
				targetName, _ := req.Args["name"].(string)
				targetID, _ := req.Args["id"].(string)
				public := " raw previous "
				empty := ""
				switch req.Cmd {
				case controlplane.CmdConfigSchema:
					value = appsettings.SchemaOutput{Sections: []core.Section{{ID: "owned", Name: "Owned", Source: "owned", Fields: []core.Field{{Env: "AGEZT_W45D_PUBLIC", Label: "Public", Type: core.TypeText, Apply: core.ApplyRestart}, {Env: "AGEZT_W45D_SECRET", Label: "Secret", Type: core.TypePassword, Secret: true, Apply: core.ApplyRestart}}}}, ReloadBoundaries: []core.ReloadBoundary{}}
				case controlplane.CmdConfigValues:
					value = appsettings.ValuesOutput{Fields: []appsettings.ValueRow{{Env: "AGEZT_W45D_PUBLIC", Set: true, EnvPinned: true, Value: &public}, {Env: "AGEZT_W45D_SECRET", Secret: true, Set: true}, {Env: "AGEZT_W45D_UNSET", Secret: true}, {Env: "AGEZT_W45D_EMPTY", Value: &empty}}}
				case controlplane.CmdConfigSet:
					value = appsettings.SetOutput{Env: targetName, Saved: true, Applied: reply.applied, EnvPinned: reply.pinned}
				case controlplane.CmdConfigSchemaRegister:
					value = appsettings.RegisterOutput{ID: "owned", Registered: true, Applied: "restart"}
				case controlplane.CmdConfigSchemaUnregister:
					value = appsettings.UnregisterOutput{ID: targetID, Removed: reply.removed}
				}
				raw, _ := json.Marshal(value)
				if decl := schemas[req.Cmd]; len(decl) > 0 {
					if err := schema.ValidateJSON(decl, raw); err != nil {
						t.Errorf("canned native result violates schema:%s:%v", req.Cmd, err)
					}
				}
				var result map[string]any
				json.Unmarshal(raw, &result)
				response := controlplane.Response{ID: req.ID, Type: controlplane.RespResult, Result: result}
				if reply.fail == req.Cmd {
					response.Type = controlplane.RespError
					response.Error = "owned failure"
					response.Result = nil
				}
				json.NewEncoder(conn).Encode(response)
			}()
		}
	}()
	t.Cleanup(func() { ln.Close(); <-done })
	return requests
}
func TestCmdSettingsNativeWireRenderingAndCheckpoints(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		reply    settingsCLIReply
		commands []string
		wire     map[string]any
		code     int
		contains string
	}{
		{"schema", []string{"schema"}, settingsCLIReply{}, []string{controlplane.CmdConfigSchema}, nil, 0, "[owned] Owned (registered)"},
		{"schema-json", []string{"schema", "--json"}, settingsCLIReply{}, []string{controlplane.CmdConfigSchema}, nil, 0, `"reload_boundaries": []`},
		{"list", []string{"ls"}, settingsCLIReply{}, []string{controlplane.CmdConfigValues}, nil, 0, "set (secret)"},
		{"list-json", []string{"ls", "--json"}, settingsCLIReply{}, []string{controlplane.CmdConfigValues}, nil, 0, `"value": ""`},
		{"get-public", []string{"get", "AGEZT_W45D_PUBLIC"}, settingsCLIReply{}, []string{controlplane.CmdConfigValues}, nil, 0, "AGEZT_W45D_PUBLIC= raw previous "},
		{"get-secret", []string{"get", "AGEZT_W45D_SECRET"}, settingsCLIReply{}, []string{controlplane.CmdConfigValues}, nil, 0, "value not shown"},
		{"get-unset", []string{"get", "AGEZT_W45D_UNSET"}, settingsCLIReply{}, []string{controlplane.CmdConfigValues}, nil, 0, "not set"},
		{"get-missing", []string{"get", "missing"}, settingsCLIReply{}, []string{controlplane.CmdConfigValues}, nil, 1, "unknown setting"},
		{"set-live", []string{"set", "AGEZT_W45D_PUBLIC", " raw ", "second"}, settingsCLIReply{applied: "live"}, []string{controlplane.CmdConfigValues, controlplane.CmdConfigSet}, map[string]any{"name": "AGEZT_W45D_PUBLIC", "value": " raw  second"}, 0, "applied live"},
		{"set-pinned", []string{"set", "AGEZT_W45D_PUBLIC", "v"}, settingsCLIReply{pinned: true, applied: "restart"}, []string{controlplane.CmdConfigValues, controlplane.CmdConfigSet}, map[string]any{"name": "AGEZT_W45D_PUBLIC", "value": "v"}, 0, "pinned by the environment"},
		{"set-clear", []string{"set", "AGEZT_W45D_PUBLIC"}, settingsCLIReply{applied: "restart"}, []string{controlplane.CmdConfigValues, controlplane.CmdConfigSet}, map[string]any{"name": "AGEZT_W45D_PUBLIC", "value": ""}, 0, "restart to apply"},
		{"set-secret", []string{"set", "AGEZT_W45D_SECRET", "owned"}, settingsCLIReply{applied: "restart"}, []string{controlplane.CmdConfigValues, controlplane.CmdConfigSet}, map[string]any{"name": "AGEZT_W45D_SECRET", "value": "owned"}, 0, "restart to apply"},
		{"register", []string{"schema", "register", "FILE"}, settingsCLIReply{}, []string{controlplane.CmdConfigSchemaRegister}, map[string]any{"section": map[string]any{"id": "owned", "name": "Owned", "fields": []any{map[string]any{"env": "AGEZT_W45D_REGISTERED", "type": "text"}}}}, 0, "registered schema section"},
		{"register-error", []string{"schema", "register", "FILE"}, settingsCLIReply{fail: controlplane.CmdConfigSchemaRegister}, []string{controlplane.CmdConfigSchemaRegister}, map[string]any{"section": map[string]any{"id": "owned", "name": "Owned", "fields": []any{map[string]any{"env": "AGEZT_W45D_REGISTERED", "type": "text"}}}}, 1, "owned failure"},
		{"unregister-force", []string{"schema", "unregister", " raw-id ", "--force"}, settingsCLIReply{removed: true}, []string{controlplane.CmdConfigSchemaUnregister}, map[string]any{"id": " raw-id ", "force": true}, 0, "unregistered"},
		{"unregister-missing", []string{"schema", "unregister", "missing"}, settingsCLIReply{}, []string{controlplane.CmdConfigSchemaUnregister}, map[string]any{"id": "missing", "force": false}, 0, "was not registered"},
		{"schema-error", []string{"schema"}, settingsCLIReply{fail: controlplane.CmdConfigSchema}, []string{controlplane.CmdConfigSchema}, nil, 1, "owned failure"},
		{"values-error", []string{"ls"}, settingsCLIReply{fail: controlplane.CmdConfigValues}, []string{controlplane.CmdConfigValues}, nil, 1, "owned failure"},
		{"checkpoint-error", []string{"set", "AGEZT_W45D_PUBLIC", "v"}, settingsCLIReply{fail: controlplane.CmdConfigValues}, []string{controlplane.CmdConfigValues}, nil, 1, "checkpoint: controlplane: owned failure"},
		{"set-error", []string{"set", "AGEZT_W45D_PUBLIC", "v"}, settingsCLIReply{fail: controlplane.CmdConfigSet}, []string{controlplane.CmdConfigValues, controlplane.CmdConfigSet}, map[string]any{"name": "AGEZT_W45D_PUBLIC", "value": "v"}, 1, "owned failure"},
		{"unregister-error", []string{"schema", "unregister", "owned"}, settingsCLIReply{fail: controlplane.CmdConfigSchemaUnregister}, []string{controlplane.CmdConfigSchemaUnregister}, map[string]any{"id": "owned", "force": false}, 1, "owned failure"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := settingsCLIEndpoint(t, tc.reply)
			args := append([]string(nil), tc.args...)
			if tc.name == "register" || tc.name == "register-error" {
				path := filepath.Join(t.TempDir(), "section.json")
				raw, _ := json.Marshal(tc.wire["section"])
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				args[2] = path
			}
			var out, errOut bytes.Buffer
			if code := cmdConfig(args, &out, &errOut); code != tc.code || !strings.Contains(out.String()+errOut.String(), tc.contains) {
				t.Fatal(args, code, out.String(), errOut.String())
			}
			if tc.name == "list" && !strings.Contains(out.String(), "[env-pinned]") {
				t.Fatal("list lost env pin", out.String())
			}
			if tc.name == "list" {
				found := 0
				for _, line := range strings.Split(out.String(), "\n") {
					switch {
					case strings.Contains(line, "AGEZT_W45D_PUBLIC"):
						found++
						if !strings.Contains(line, " raw previous ") || !strings.Contains(line, "[env-pinned]") {
							t.Fatal(line)
						}
					case strings.Contains(line, "AGEZT_W45D_SECRET"):
						found++
						if !strings.Contains(line, "set (secret)") {
							t.Fatal(line)
						}
					case strings.Contains(line, "AGEZT_W45D_UNSET"):
						found++
						if !strings.Contains(line, "unset (secret)") {
							t.Fatal(line)
						}
					case strings.Contains(line, "AGEZT_W45D_EMPTY"):
						found++
						if !strings.Contains(line, "unset") {
							t.Fatal(line)
						}
					}
				}
				if found != 4 {
					t.Fatal("missing settings rows", out.String())
				}
			}
			expected := append([]string{controlplane.CmdStatus}, tc.commands...)
			for _, command := range expected {
				select {
				case req := <-requests:
					if req.Cmd != command || req.Token != "owned-token" {
						t.Fatal(req, command)
					}
					if command == tc.commands[len(tc.commands)-1] && !reflect.DeepEqual(req.Args, tc.wire) {
						t.Fatal(req, tc.wire)
					}
				case <-time.After(time.Second):
					t.Fatal("missing request", command)
				}
			}
			if len(requests) != 0 {
				t.Fatal("extra requests")
			}
			if strings.HasPrefix(tc.name, "set-") && tc.name != "checkpoint-error" {
				catalog, err := loadRollbackCatalog()
				if err != nil || len(catalog.Checkpoints) != 1 {
					t.Fatal(catalog, err)
				}
				cp := catalog.Checkpoints[0]
				if cp.Kind != rollbackCheckpointKindConfig || cp.Action != "config.set" || cp.SubjectID != args[1] {
					t.Fatal(cp)
				}
				if tc.name == "set-secret" {
					if cp.Before["rollbackable"] != false {
						t.Fatal(cp)
					}
					if _, ok := cp.Before["value"]; ok {
						t.Fatal("secret checkpoint has value")
					}
				} else if cp.Before["value"] != " raw previous " {
					t.Fatal(cp)
				}
			}
		})
	}
}
func TestCmdSettingsNativeInvalidHelpAndMalformedFilesDoNotDial(t *testing.T) {
	requests := settingsCLIEndpoint(t, settingsCLIReply{})
	for _, args := range [][]string{{"schema", "unexpected"}, {"ls", "unexpected"}, {"get"}, {"get", "one", "two"}, {"set"}, {"schema", "register"}, {"schema", "register", "one", "two"}, {"schema", "unregister"}, {"schema", "unregister", "one", "two"}, {"schema", "--help"}, {"ls", "--help"}, {"get", "--help"}, {"set", "--help"}, {"schema", "register", "--help"}, {"schema", "unregister", "--help"}} {
		var out, errOut bytes.Buffer
		expected := 2
		if args[len(args)-1] == "--help" {
			expected = 0
		}
		if code := cmdConfig(args, &out, &errOut); code != expected {
			t.Fatal(args, code, out.String(), errOut.String())
		}
	}
	for _, content := range []string{`{broken`, `[]`} {
		path := filepath.Join(t.TempDir(), "invalid.json")
		os.WriteFile(path, []byte(content), 0600)
		var out, errOut bytes.Buffer
		if code := cmdConfig([]string{"schema", "register", path}, &out, &errOut); code != 1 {
			t.Fatal(code, out.String(), errOut.String())
		}
	}
	select {
	case req := <-requests:
		t.Fatal("validation dialed", req)
	default:
	}
}
