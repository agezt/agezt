// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func mcpListFixture(t *testing.T) (*runtime.Kernel, *Server, *mock.Provider) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	rows := []mcp.Server{{ID: "owned", Name: "owned", Command: "never-executed", CreatedMS: 9007199254740993, UpdatedMS: 9223372036854775807, Env: map[string]string{"OWNED": "owned-private-value"}, Headers: map[string]string{"Owned": "owned-private-header"}}}
	raw, _ := json.Marshal(rows)
	if err := os.WriteFile(filepath.Join(path, "servers.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	p := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: p})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	s := NewServer(k, dir)
	s.token = "primary"
	return k, s, p
}
func mcpListReply(t *testing.T, ctx context.Context, s *Server) map[string]any {
	t.Helper()
	client, conn := net.Pipe()
	defer client.Close()
	defer conn.Close()
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(ctx, conn) }()
	client.SetDeadline(time.Now().Add(3 * time.Second))
	raw, _ := json.Marshal(Request{ID: "owned", Cmd: CmdMCPList, Token: "primary", Args: map[string]any{"tenant": "spoof", "unknown": true}})
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
	var reply map[string]any
	if err := dec.Decode(&reply); err != nil {
		t.Fatal(err)
	}
	return reply
}
func TestMCPListNativeLargeTimestampsRemainExactAndRedacted(t *testing.T) {
	k, s, p := mcpListFixture(t)
	head, hash := k.Journal().Head()
	reply := mcpListReply(t, context.Background(), s)
	if reply["type"] != RespResult {
		t.Fatal(reply)
	}
	row := reply["result"].(map[string]any)["servers"].([]any)[0].(map[string]any)
	for key, want := range map[string]string{"created_ms": "9007199254740993", "updated_ms": "9223372036854775807"} {
		if got, ok := row[key].(json.Number); !ok || got.String() != want {
			t.Fatalf("%s EXPECTED:%s ACTUAL:%v", key, want, row[key])
		}
	}
	serialized, _ := json.Marshal(row)
	if strings.Contains(string(serialized), "owned-private-value") || strings.Contains(string(serialized), "owned-private-header") {
		t.Fatal("private value leaked")
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatal(head, after, p.CallCount())
	}
}
func TestMCPListNativeCanceledAdmissionRejectsBeforeRead(t *testing.T) {
	k, s, p := mcpListFixture(t)
	head, hash := k.Journal().Head()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reply := mcpListReply(t, ctx, s)
	message, _ := reply["error"].(string)
	after, afterHash := k.Journal().Head()
	if reply["type"] != RespError || !strings.Contains(message, "context canceled") || head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatalf("EXPECTED:canceled admission ACTUAL:type=%v error=%q", reply["type"], message)
	}
}

func TestMCPListNativeReadPolicyComesFromOneTypedOperation(t *testing.T) {
	seen := 0
	for _, op := range registeredAppOperations() {
		spec := op.Spec()
		if spec.Name != CmdMCPList {
			continue
		}
		wire, ok := commandRegistry[spec.Name]
		if !ok || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput || spec.Input != reflect.TypeFor[apptools.MCPListInput]() || spec.Output != reflect.TypeFor[apptools.MCPListOutput]() {
			t.Fatal(spec, wire)
		}
		seen++
	}
	if seen != 1 || len(mcpCatalogOperations) != 1 {
		t.Fatal(seen)
	}
}
