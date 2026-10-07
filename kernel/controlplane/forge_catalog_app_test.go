// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/toolforge"
	"github.com/agezt/agezt/plugins/providers/mock"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func forgeReadFixture(t *testing.T) (*runtime.Kernel, *Server, *mock.Provider) {
	t.Helper()
	dir := t.TempDir()
	tfdir := filepath.Join(dir, "toolforge")
	if err := os.MkdirAll(tfdir, 0700); err != nil {
		t.Fatal(err)
	}
	rows := []toolforge.ScriptTool{{ID: "large", Name: "large", Code: "never executed", Status: toolforge.StatusActive, TestedMS: 9007199254740993, CreatedMS: 9007199254740993, UpdatedMS: 9223372036854775807}}
	raw, _ := json.Marshal(rows)
	if err := os.WriteFile(filepath.Join(tfdir, "scripttools.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	provider := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	s := NewServer(k, dir)
	s.token = "primary"
	return k, s, provider
}
func forgeNativeReply(t *testing.T, ctx context.Context, s *Server, cmd string) map[string]any {
	t.Helper()
	client, conn := net.Pipe()
	defer client.Close()
	defer conn.Close()
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(ctx, conn) }()
	client.SetDeadline(time.Now().Add(3 * time.Second))
	raw, _ := json.Marshal(Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"ref": "large", "unused": true}})
	if _, err := client.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(client).ReadBytes(10)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	<-done
	dec := json.NewDecoder(strings.NewReader(string(line)))
	dec.UseNumber()
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestForgeCatalogNativeLargeTimestampsRemainExact(t *testing.T) {
	for _, cmd := range []string{CmdToolforgeList, CmdToolforgeShow} {
		t.Run(cmd, func(t *testing.T) {
			k, s, p := forgeReadFixture(t)
			head, hash := k.Journal().Head()
			reply := forgeNativeReply(t, context.Background(), s, cmd)
			if reply["type"] != RespResult {
				t.Fatal(reply)
			}
			result := reply["result"].(map[string]any)
			var row map[string]any
			if cmd == CmdToolforgeList {
				row = result["tools"].([]any)[0].(map[string]any)
			} else {
				row = result["tool"].(map[string]any)
			}
			for key, want := range map[string]string{"created_ms": "9007199254740993", "tested_ms": "9007199254740993", "updated_ms": "9223372036854775807"} {
				if got, ok := row[key].(json.Number); !ok || got.String() != want {
					t.Fatalf("%s EXPECTED:%s ACTUAL:%v", key, want, row[key])
				}
			}
			after, afterHash := k.Journal().Head()
			if head != after || hash != afterHash || p.CallCount() != 0 {
				t.Fatal(head, after, p.CallCount())
			}
		})
	}
}
func TestForgeCatalogNativeCanceledAdmissionStopsBeforeRead(t *testing.T) {
	for _, cmd := range []string{CmdToolforgeList, CmdToolforgeShow} {
		t.Run(cmd, func(t *testing.T) {
			_, s, _ := forgeReadFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			reply := forgeNativeReply(t, ctx, s, cmd)
			message, _ := reply["error"].(string)
			if reply["type"] != RespError || !strings.Contains(message, "context canceled") {
				t.Fatalf("EXPECTED:canceled admission ACTUAL:%v", reply)
			}
		})
	}
}

func TestForgeCatalogNativePoliciesComeFromTwoTypedPrimaryReads(t *testing.T) {
	want := map[string]reflect.Type{CmdToolforgeList: reflect.TypeFor[apptools.ForgeListOutput](), CmdToolforgeShow: reflect.TypeFor[apptools.ForgeShowOutput]()}
	seen := map[string]bool{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		output, known := want[spec.Name]
		if !known {
			continue
		}
		wire, ok := commandRegistry[spec.Name]
		input := reflect.TypeFor[apptools.ForgeListInput]()
		if spec.Name == CmdToolforgeShow {
			input = reflect.TypeFor[apptools.ForgeShowRequest]()
		}
		if seen[spec.Name] || !ok || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput || spec.Input != input || spec.Output != output {
			t.Fatal(spec, wire)
		}
		seen[spec.Name] = true
	}
	if len(seen) != 2 || len(forgeReadOperations) != 2 {
		t.Fatal(seen, len(forgeReadOperations))
	}
}
