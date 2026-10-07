// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
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

type forgeCallerKey struct{}
type forgeBindingRunner struct {
	ctx   context.Context
	calls int
	block bool
}

func (r *forgeBindingRunner) RunScript(ctx context.Context, _, _, _ string) (string, bool, error) {
	r.ctx = ctx
	r.calls++
	if r.block {
		<-ctx.Done()
		return "", false, ctx.Err()
	}
	return "owned output", false, nil
}
func forgeMutationCommands() []string {
	return []string{CmdToolforgeDraft, CmdToolforgeEdit, CmdToolforgeTest, CmdToolforgePromote, CmdToolforgeQuarantine, CmdToolforgeRemove}
}
func forgeMutationFixture(t *testing.T, cmd string) (*runtime.Kernel, *Server, *forgeBindingRunner, *mock.Provider, map[string]any, string) {
	t.Helper()
	dir := t.TempDir()
	forgeDir := filepath.Join(dir, "toolforge")
	if err := os.MkdirAll(forgeDir, 0700); err != nil {
		t.Fatal(err)
	}
	status := toolforge.StatusDraft
	if cmd == CmdToolforgeQuarantine {
		status = toolforge.StatusActive
	}
	rows := []toolforge.ScriptTool{{ID: "owned", Name: "owned", Description: "owned fixture", Language: "python", Code: "never executed", Status: status, TestedOK: true, CreatedMS: 100, UpdatedMS: 100}}
	raw, _ := json.Marshal(rows)
	file := filepath.Join(forgeDir, "scripttools.json")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	runner := &forgeBindingRunner{}
	provider := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: provider, ScriptRunner: runner})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	s := NewServer(k, dir)
	s.token = "primary"
	args := map[string]any{"ref": "owned", "unused": true, "correlation_id": "caller-poison", "tenant": "spoof"}
	switch cmd {
	case CmdToolforgeDraft:
		args["tool"] = map[string]any{"name": "new", "description": "owned", "language": "python", "code": "never executed"}
	case CmdToolforgeEdit:
		args["tool"] = map[string]any{"description": "edited"}
	case CmdToolforgeTest:
		args["input"] = `{"x":1}`
	case CmdToolforgeQuarantine:
		args["reason"] = " owned reason "
	}
	return k, s, runner, provider, args, file
}
func forgeMutationReply(t *testing.T, ctx context.Context, s *Server, cmd string, args map[string]any) Response {
	t.Helper()
	client, conn := net.Pipe()
	defer client.Close()
	defer conn.Close()
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(ctx, conn) }()
	client.SetDeadline(time.Now().Add(3 * time.Second))
	raw, _ := json.Marshal(Request{ID: cmd, Cmd: cmd, Token: "primary", Args: args})
	if _, err := client.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(client).ReadBytes(10)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	<-done
	var response Response
	if err := json.Unmarshal(line, &response); err != nil {
		t.Fatal(err)
	}
	return response
}
func TestForgeLifecycleNativeAuditFailureBlocksAllSixEffects(t *testing.T) {
	for _, cmd := range forgeMutationCommands() {
		t.Run(cmd, func(t *testing.T) {
			k, s, r, p, args, file := forgeMutationFixture(t, cmd)
			before := k.ToolForge().List()
			disk, _ := os.ReadFile(file)
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
			reply := forgeMutationReply(t, context.Background(), s, cmd, args)
			after, _ := os.ReadFile(file)
			if reply.Type != RespError || !strings.Contains(reply.Error, "journal") || !reflect.DeepEqual(before, k.ToolForge().List()) || !bytes.Equal(disk, after) || r.calls != 0 || p.CallCount() != 0 {
				t.Fatalf("EXPECTED:audit failure blocks effects ACTUAL:reply=%+v changed=%v runner=%d", reply, !reflect.DeepEqual(before, k.ToolForge().List()), r.calls)
			}
		})
	}
}
func TestForgeLifecycleNativeCanceledAdmissionBlocksAllSixEffects(t *testing.T) {
	for _, cmd := range forgeMutationCommands() {
		t.Run(cmd, func(t *testing.T) {
			k, s, r, p, args, file := forgeMutationFixture(t, cmd)
			before := k.ToolForge().List()
			disk, _ := os.ReadFile(file)
			head, hash := k.Journal().Head()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			reply := forgeMutationReply(t, ctx, s, cmd, args)
			after, _ := os.ReadFile(file)
			seq, afterHash := k.Journal().Head()
			if reply.Type != RespError || !strings.Contains(reply.Error, "context canceled") || !reflect.DeepEqual(before, k.ToolForge().List()) || !bytes.Equal(disk, after) || r.calls != 0 || p.CallCount() != 0 || head != seq || hash != afterHash {
				t.Fatalf("EXPECTED:canceled admission has no effects ACTUAL:reply=%+v changed=%v runner=%d seq=%d/%d", reply, !reflect.DeepEqual(before, k.ToolForge().List()), r.calls, head, seq)
			}
		})
	}
}
func TestForgeLifecycleNativeOneCorrelationAndOneAuditPair(t *testing.T) {
	want := map[string]event.Kind{CmdToolforgeDraft: event.KindScriptToolCreated, CmdToolforgeEdit: event.KindScriptToolUpdated, CmdToolforgeTest: event.KindScriptToolTested, CmdToolforgePromote: event.KindScriptToolPromoted, CmdToolforgeQuarantine: event.KindScriptToolQuarantined, CmdToolforgeRemove: event.KindScriptToolRemoved}
	for _, cmd := range forgeMutationCommands() {
		t.Run(cmd, func(t *testing.T) {
			k, s, _, p, args, _ := forgeMutationFixture(t, cmd)
			head, _ := k.Journal().Head()
			reply := forgeMutationReply(t, context.Background(), s, cmd, args)
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
			if len(events) != 3 || events[0].Kind != event.KindOpInvoked || events[1].Kind != want[cmd] || events[2].Kind != event.KindOpCompleted {
				t.Fatal(events)
			}
			corr := events[0].CorrelationID
			if corr == "" || corr == "caller-poison" || events[1].CorrelationID != corr || events[2].CorrelationID != corr || p.CallCount() != 0 {
				t.Fatalf("EXPECTED:one owned correlation ACTUAL:op=%q domain=%q terminal=%q", corr, events[1].CorrelationID, events[2].CorrelationID)
			}
		})
	}
}
func TestForgeLifecycleNativeRunnerContextRetainsValuesDeadlineIdentity(t *testing.T) {
	k, s, r, _, args, _ := forgeMutationFixture(t, CmdToolforgeTest)
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), forgeCallerKey{}, "owned-value"), time.Second)
	defer cancel()
	reply := forgeMutationReply(t, ctx, s, CmdToolforgeTest, args)
	if reply.Type != RespResult || r.calls != 1 || r.ctx == nil || r.ctx.Value(forgeCallerKey{}) != "owned-value" {
		t.Fatalf("EXPECTED:caller values reach runner ACTUAL:reply=%+v calls=%d", reply, r.calls)
	}
	expected, _ := ctx.Deadline()
	actual, ok := r.ctx.Deadline()
	if !ok || actual != expected || opapi.CorrelationFromContext(r.ctx) == "" {
		t.Fatal(actual, expected, ok)
	}
	rows, _ := k.Journal().Tail(3)
	if rows[0].CorrelationID != opapi.CorrelationFromContext(r.ctx) {
		t.Fatal(rows[0], opapi.CorrelationFromContext(r.ctx))
	}
}

func TestForgeLifecycleNativeRegistryComesFromAllEightTypedOperations(t *testing.T) {
	want := map[string]bool{CmdToolforgeList: true, CmdToolforgeShow: true}
	for _, cmd := range forgeMutationCommands() {
		want[cmd] = false
	}
	seen := map[string]bool{}
	for _, op := range registeredAppOperations() {
		spec := op.Spec()
		read, known := want[spec.Name]
		if !known {
			continue
		}
		wire, ok := commandRegistry[spec.Name]
		if !ok || seen[spec.Name] || !wire.AppOwned || wire.ReadOnly != read || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || spec.ReadOnly != read || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput {
			t.Fatal(spec, wire)
		}
		if !read {
			output := reflect.TypeFor[apptools.ForgeMutationOutput]()
			if spec.Name == CmdToolforgeTest {
				output = reflect.TypeFor[apptools.ForgeTestOutput]()
			} else if spec.Name == CmdToolforgeRemove {
				output = reflect.TypeFor[apptools.ForgeRemoveOutput]()
			}
			if spec.Output != output {
				t.Fatal(spec)
			}
		}
		seen[spec.Name] = true
	}
	if len(seen) != 8 || len(forgeLifecycleOperations) != 6 || len(forgeReadOperations) != 2 {
		t.Fatal(seen)
	}
}

func TestForgeLifecycleNativeRunnerDeadlineLeavesTestRecordUnchanged(t *testing.T) {
	k, s, r, _, args, file := forgeMutationFixture(t, CmdToolforgeTest)
	r.block = true
	before := k.ToolForge().List()
	disk, _ := os.ReadFile(file)
	head, _ := k.Journal().Head()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	reply := forgeMutationReply(t, ctx, s, CmdToolforgeTest, args)
	after, _ := os.ReadFile(file)
	if reply.Type != RespError || !strings.Contains(reply.Error, "context deadline exceeded") || r.calls != 1 || !reflect.DeepEqual(before, k.ToolForge().List()) || !bytes.Equal(disk, after) {
		t.Fatal(reply, r.calls, before, k.ToolForge().List())
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
	if len(events) != 2 || events[0].Kind != event.KindOpInvoked || events[1].Kind != event.KindOpFailed || events[0].CorrelationID == "" || events[0].CorrelationID != events[1].CorrelationID {
		t.Fatal(events)
	}
}
