// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
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

type mcpLifecycleCallerKey struct{}
type mcpLifecycleNativePeer struct {
	closed bool
	calls  int
}

func (p *mcpLifecycleNativePeer) Tools() []mcp.ToolDef { return []mcp.ToolDef{{Name: "owned"}} }
func (p *mcpLifecycleNativePeer) Call(context.Context, string, json.RawMessage) (string, bool, error) {
	p.calls++
	return "unused", false, nil
}
func (p *mcpLifecycleNativePeer) Close() error { p.closed = true; return nil }

type mcpLifecycleNativeDial struct {
	block bool
	calls int
	ctx   context.Context
	peer  *mcpLifecycleNativePeer
}

func (d *mcpLifecycleNativeDial) dial(ctx context.Context) (mcp.Conn, error) {
	d.calls++
	d.ctx = ctx
	if d.block {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return nil, errors.New("owned dial did not inherit cancellation")
		}
	}
	return d.peer, nil
}
func mcpLifecycleNativeCommands() []string {
	return []string{CmdMCPAdd, CmdMCPAttach, CmdMCPDetach, CmdMCPSetEnabled, CmdMCPRemove}
}
func mcpLifecycleNativeFixture(t *testing.T, cmd string) (*runtime.Kernel, *Server, *mcpLifecycleNativeDial, *mock.Provider, map[string]any, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	rows := []mcp.Server{{ID: "owned", Name: "owned", Command: "never-executed", Enabled: true, CreatedMS: 11, UpdatedMS: 22, Env: map[string]string{"OWNED": "owned-private-env"}}}
	raw, _ := json.Marshal(rows)
	file := filepath.Join(path, "servers.json")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	d := &mcpLifecycleNativeDial{peer: &mcpLifecycleNativePeer{}}
	p := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: p, MCPDialer: func(ctx context.Context, _ string, _ []string, _ map[string]string) (mcp.Conn, error) {
		return d.dial(ctx)
	}, MCPHTTPDialer: func(ctx context.Context, _ string, _ map[string]string) (mcp.Conn, error) { return d.dial(ctx) }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	if cmd == CmdMCPDetach || cmd == CmdMCPRemove {
		if _, _, err := k.AttachMCPServer(context.Background(), "seed", "owned"); err != nil {
			t.Fatal(err)
		}
	}
	d.calls = 0
	d.ctx = nil
	s := NewServer(k, dir)
	s.token = "primary"
	args := map[string]any{"ref": "owned", "enabled": false, "unknown": true, "tenant": "spoof", "correlation_id": "caller-poison"}
	if cmd == CmdMCPAdd {
		args["server"] = map[string]any{"name": "new", "command": "never-executed", "env": map[string]any{"OWNED": "owned-private-env"}}
	}
	return k, s, d, p, args, file
}
func mcpLifecycleNativeReply(t *testing.T, ctx context.Context, s *Server, cmd string, args map[string]any) Response {
	t.Helper()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(ctx, b) }()
	raw, _ := json.Marshal(Request{ID: "owned", Cmd: cmd, Token: "primary", Args: args})
	if _, err := a.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(a).ReadBytes(10)
	if err != nil {
		t.Fatal(err)
	}
	a.Close()
	<-done
	if strings.Contains(string(line), "owned-private-env") {
		t.Fatal("private registration value leaked")
	}
	var reply Response
	if err := json.Unmarshal(line, &reply); err != nil {
		t.Fatal(err)
	}
	return reply
}
func TestMCPLifecycleNativeAuditFailureBlocksAllFiveEffects(t *testing.T) {
	for _, cmd := range mcpLifecycleNativeCommands() {
		t.Run(cmd, func(t *testing.T) {
			k, s, d, p, args, file := mcpLifecycleNativeFixture(t, cmd)
			rows := k.MCPStore().List()
			attached := k.MCPAttached()
			disk, _ := os.ReadFile(file)
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
			reply := mcpLifecycleNativeReply(t, context.Background(), s, cmd, args)
			after, _ := os.ReadFile(file)
			if reply.Type != RespError || !strings.Contains(reply.Error, "journal") || !reflect.DeepEqual(rows, k.MCPStore().List()) || !reflect.DeepEqual(attached, k.MCPAttached()) || !bytes.Equal(disk, after) || d.calls != 0 || d.peer.closed || d.peer.calls != 0 || p.CallCount() != 0 {
				t.Fatalf("EXPECTED:audit admission blocks every effect ACTUAL:reply=%+v dial=%d closed=%v", reply, d.calls, d.peer.closed)
			}
		})
	}
}
func TestMCPLifecycleNativeCanceledAdmissionBlocksAllFiveEffects(t *testing.T) {
	for _, cmd := range mcpLifecycleNativeCommands() {
		t.Run(cmd, func(t *testing.T) {
			k, s, d, p, args, file := mcpLifecycleNativeFixture(t, cmd)
			rows := k.MCPStore().List()
			attached := k.MCPAttached()
			disk, _ := os.ReadFile(file)
			seq, hash := k.Journal().Head()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			reply := mcpLifecycleNativeReply(t, ctx, s, cmd, args)
			after, _ := os.ReadFile(file)
			newSeq, newHash := k.Journal().Head()
			if reply.Type != RespError || !strings.Contains(reply.Error, "context canceled") || !reflect.DeepEqual(rows, k.MCPStore().List()) || !reflect.DeepEqual(attached, k.MCPAttached()) || !bytes.Equal(disk, after) || d.calls != 0 || d.peer.closed || d.peer.calls != 0 || p.CallCount() != 0 || seq != newSeq || hash != newHash {
				t.Fatalf("EXPECTED:canceled admission blocks every effect ACTUAL:reply=%+v dial=%d closed=%v seq=%d/%d", reply, d.calls, d.peer.closed, seq, newSeq)
			}
		})
	}
}
func TestMCPLifecycleNativeOneOwnedCorrelationAndAuditPair(t *testing.T) {
	kinds := map[string]event.Kind{CmdMCPAdd: event.KindMCPAdded, CmdMCPAttach: event.KindMCPAttached, CmdMCPDetach: event.KindMCPDetached, CmdMCPSetEnabled: event.KindMCPUpdated, CmdMCPRemove: event.KindMCPRemoved}
	for _, cmd := range mcpLifecycleNativeCommands() {
		t.Run(cmd, func(t *testing.T) {
			k, s, _, p, args, _ := mcpLifecycleNativeFixture(t, cmd)
			head, _ := k.Journal().Head()
			reply := mcpLifecycleNativeReply(t, context.Background(), s, cmd, args)
			if reply.Type != RespResult {
				t.Fatal(reply)
			}
			rows, err := k.Journal().Tail(100)
			if err != nil {
				t.Fatal(err)
			}
			var events []*event.Event
			for _, e := range rows {
				if e.Seq > head {
					events = append(events, e)
				}
			}
			want := 3
			if cmd == CmdMCPRemove {
				want = 4
			}
			if len(events) != want || events[0].Kind != event.KindOpInvoked || events[len(events)-1].Kind != event.KindOpCompleted || events[len(events)-2].Kind != kinds[cmd] {
				t.Fatal(events)
			}
			corr := events[0].CorrelationID
			for _, e := range events {
				if corr == "" || corr == "caller-poison" || e.CorrelationID != corr || p.CallCount() != 0 {
					t.Fatalf("EXPECTED:one host-owned identity ACTUAL:op=%q domain=%q", corr, e.CorrelationID)
				}
			}
		})
	}
}
func TestMCPLifecycleNativeAttachRetainsCallerContext(t *testing.T) {
	k, s, d, _, args, _ := mcpLifecycleNativeFixture(t, CmdMCPAttach)
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), mcpLifecycleCallerKey{}, "owned"), time.Second)
	defer cancel()
	reply := mcpLifecycleNativeReply(t, ctx, s, CmdMCPAttach, args)
	if reply.Type != RespResult || d.calls != 1 || d.ctx == nil || d.ctx.Value(mcpLifecycleCallerKey{}) != "owned" {
		t.Fatalf("EXPECTED:caller context reaches dial ACTUAL:reply=%+v dial=%d ctx=%v", reply, d.calls, d.ctx)
	}
	want, _ := ctx.Deadline()
	actual, ok := d.ctx.Deadline()
	if !ok || actual != want || opapi.CorrelationFromContext(d.ctx) == "" {
		t.Fatal(actual, ok, d.ctx)
	}
	rows, _ := k.Journal().Tail(100)
	found := false
	for _, e := range rows {
		if e.Kind == event.KindMCPAttached {
			found = true
			if e.CorrelationID != opapi.CorrelationFromContext(d.ctx) {
				t.Fatal(e)
			}
		}
	}
	if !found {
		t.Fatal("attachment event missing")
	}
}

func TestMCPLifecycleNativeAttachDeadlineStopsBeforeAttachment(t *testing.T) {
	k, s, d, p, args, file := mcpLifecycleNativeFixture(t, CmdMCPAttach)
	before := k.MCPStore().List()
	disk, _ := os.ReadFile(file)
	head, _ := k.Journal().Head()
	d.block = true
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	reply := mcpLifecycleNativeReply(t, ctx, s, CmdMCPAttach, args)
	after, _ := os.ReadFile(file)
	if reply.Type != RespError || !strings.Contains(reply.Error, "context deadline exceeded") || d.calls != 1 || d.peer.closed || !reflect.DeepEqual(before, k.MCPStore().List()) || !bytes.Equal(disk, after) || len(k.MCPAttached()) != 0 || p.CallCount() != 0 {
		t.Fatal(reply, d.calls, k.MCPAttached())
	}
	rows, _ := k.Journal().Tail(100)
	var events []*event.Event
	for _, e := range rows {
		if e.Seq > head {
			events = append(events, e)
		}
	}
	if len(events) != 2 || events[0].Kind != event.KindOpInvoked || events[1].Kind != event.KindOpFailed || events[0].CorrelationID == "" || events[0].CorrelationID != events[1].CorrelationID {
		t.Fatal(events)
	}
}
func TestMCPLifecycleNativePolicyComesFromFiveTypedOperations(t *testing.T) {
	inputs := map[string]reflect.Type{CmdMCPAdd: reflect.TypeFor[apptools.MCPAddRequest](), CmdMCPAttach: reflect.TypeFor[apptools.MCPRefRequest](), CmdMCPDetach: reflect.TypeFor[apptools.MCPRefRequest](), CmdMCPSetEnabled: reflect.TypeFor[apptools.MCPSetEnabledRequest](), CmdMCPRemove: reflect.TypeFor[apptools.MCPRefRequest]()}
	seen := map[string]int{}
	for _, op := range registeredAppOperations() {
		sp := op.Spec()
		input, ok := inputs[sp.Name]
		if !ok {
			continue
		}
		wire, ok := commandRegistry[sp.Name]
		if !ok || !wire.AppOwned || wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || sp.ReadOnly || sp.Authz != opapi.PrimaryOnly || sp.Tenancy != opapi.Primary || sp.Stream != opapi.StreamNone || !sp.AllowUnknownInput || sp.Input != input {
			t.Fatal(sp, wire)
		}
		seen[sp.Name]++
	}
	if len(mcpLifecycleOperations) != 5 || len(seen) != 5 {
		t.Fatal(seen)
	}
	for _, count := range seen {
		if count != 1 {
			t.Fatal(seen)
		}
	}
}
