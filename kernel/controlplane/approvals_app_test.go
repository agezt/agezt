// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestApprovalHistoryNativeTypedRegistry(t *testing.T) {
	for _, cmd := range []string{CmdApprovalsLog, CmdApprovalsStats} {
		found := 0
		for _, operation := range registeredAppOperations() {
			spec := operation.Spec()
			if spec.Name != cmd {
				continue
			}
			found++
			if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || !spec.ReadOnly || !spec.AllowUnknownInput || spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant {
				t.Fatalf("%s metadata=%+v", cmd, spec)
			}
		}
		wire, exists := commandRegistry[cmd]
		if found != 1 || !exists || !wire.AppOwned || !wire.ReadOnly || !wire.TenantAllowed || !wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire found=%d metadata=%+v", cmd, found, wire)
		}
	}
}

// Each token reads the approval history journaled by the kernel it is routed
// to, on the daemon clock.
func TestApprovalHistoryNativeRouting(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	reg, err := tenant.New(filepath.Join(dir, "tenants"), func(_ string, baseDir string) (io.Closer, error) {
		return runtime.Open(runtime.Config{BaseDir: baseDir, Provider: mock.New()})
	})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.CloseAll()
	entry, err := reg.Acquire("acme", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tk := entry.Kernel.(*runtime.Kernel)
	tenantToken, _ := reg.Token("acme")
	publish := func(kk *runtime.Kernel, kind event.Kind, payload map[string]any) {
		if _, err := kk.Bus().Publish(event.Spec{Subject: "approval", Kind: kind, Actor: "agent", Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	publish(k, event.KindApprovalRequested, map[string]any{"approval_id": "primary-ap", "capability": "shell"})
	publish(tk, event.KindApprovalRequested, map[string]any{"approval_id": "tenant-ap", "capability": "shell"})
	publish(tk, event.KindApprovalDenied, map[string]any{"approval_id": "tenant-ap", "resolved_by": "lead"})
	s := NewServer(k, dir)
	s.token = "primary"
	s.SetTenants(reg)
	call := func(token, cmd string, args map[string]any) string {
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(3 * time.Second))
		done := make(chan struct{})
		go func() { defer close(done); s.handleConn(context.Background(), b) }()
		raw, _ := json.Marshal(Request{ID: "e", Cmd: cmd, Token: token, Args: args})
		if _, err := a.Write(append(raw, 10)); err != nil {
			t.Fatal(err)
		}
		line, err := bufio.NewReader(a).ReadBytes(10)
		a.Close()
		<-done
		if err != nil {
			t.Fatal(err)
		}
		return string(line)
	}
	for _, c := range []struct {
		token, cmd string
		args       map[string]any
		want       string
	}{
		{"primary", CmdApprovalsLog, nil, `"approval_id":"primary-ap"`},
		{"primary", CmdApprovalsLog, map[string]any{"tenant": " acme "}, `"resolved_by":"lead"`},
		{tenantToken, CmdApprovalsLog, map[string]any{"tenant": "acme", "denied": true}, `"approval_id":"tenant-ap"`},
		{"primary", CmdApprovalsStats, nil, `"pending":1`},
		{tenantToken, CmdApprovalsStats, map[string]any{"tenant": "acme", "since_ms": 600000}, `"denied":1`},
		{"primary", CmdApprovalsLog, map[string]any{"denied": "yes"}, `"error":"args.denied must be a boolean"`},
	} {
		if line := call(c.token, c.cmd, c.args); !strings.Contains(line, c.want) {
			t.Fatalf("%s %v: %s", c.cmd, c.args, line)
		}
	}
	if line := call(tenantToken, CmdApprovalsLog, map[string]any{"tenant": "acme"}); strings.Contains(line, "primary-ap") {
		t.Fatal("a tenant reads only its own approvals", line)
	}
	// A 1 ms window excludes every approval only once the clock has moved past
	// the newest one.
	newest, err := k.Journal().Tail(1)
	if err != nil || len(newest) != 1 {
		t.Fatal(newest, err)
	}
	for time.Now().UnixMilli() <= newest[0].TSUnixMS+1 {
		time.Sleep(time.Millisecond)
	}
	if line := call("primary", CmdApprovalsStats, map[string]any{"since_ms": 1}); !strings.Contains(line, `"total":0`) || !strings.Contains(line, `"window_ms":1`) {
		t.Fatal("the window follows the daemon clock", line)
	}
}

func TestApprovalLiveNativeTypedRegistry(t *testing.T) {
	for cmd, readOnly := range map[string]bool{CmdApprovals: true, CmdDecide: false} {
		found := 0
		for _, operation := range registeredAppOperations() {
			spec := operation.Spec()
			if spec.Name != cmd {
				continue
			}
			found++
			if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || spec.ReadOnly != readOnly || !spec.AllowUnknownInput || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
				t.Fatalf("%s metadata=%+v", cmd, spec)
			}
		}
		wire, exists := commandRegistry[cmd]
		if found != 1 || !exists || !wire.AppOwned || wire.ReadOnly != readOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire found=%d metadata=%+v", cmd, found, wire)
		}
	}
}

// The operator lists and resolves the primary kernel's waiting requests; a
// tenant token is refused both.
func TestApprovalLiveNativeDecide(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	reg, err := tenant.New(filepath.Join(dir, "tenants"), func(_ string, baseDir string) (io.Closer, error) {
		return runtime.Open(runtime.Config{BaseDir: baseDir, Provider: mock.New()})
	})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.CloseAll()
	if _, err := reg.Acquire("acme", time.Now()); err != nil {
		t.Fatal(err)
	}
	tenantToken, _ := reg.Token("acme")
	outcome := make(chan approval.Outcome, 1)
	go func() {
		outcome <- k.Approvals().Submit(context.Background(), approval.SubmitSpec{Capability: "shell", ToolName: "shell", CorrelationID: "corr-live"})
	}()
	for k.Approvals().PendingCount() != 1 {
		time.Sleep(time.Millisecond)
	}
	id := k.Approvals().Pending()[0].ID
	s := NewServer(k, dir)
	s.token = "primary"
	s.SetTenants(reg)
	call := func(token, cmd string, args map[string]any) string {
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(3 * time.Second))
		done := make(chan struct{})
		go func() { defer close(done); s.handleConn(context.Background(), b) }()
		raw, _ := json.Marshal(Request{ID: "e", Cmd: cmd, Token: token, Args: args})
		if _, err := a.Write(append(raw, 10)); err != nil {
			t.Fatal(err)
		}
		line, err := bufio.NewReader(a).ReadBytes(10)
		a.Close()
		<-done
		if err != nil {
			t.Fatal(err)
		}
		return string(line)
	}
	if line := call(tenantToken, CmdApprovals, map[string]any{"tenant": "acme"}); !strings.Contains(line, `"type":"error"`) {
		t.Fatal("a tenant token may not list the operator's approvals", line)
	}
	if line := call(tenantToken, CmdDecide, map[string]any{"tenant": "acme", "id": id, "decision": "grant"}); !strings.Contains(line, `"type":"error"`) || k.Approvals().PendingCount() != 1 {
		t.Fatal("a tenant token may not decide", line)
	}
	if line := call("primary", CmdApprovals, nil); !strings.Contains(line, `"id":"`+id+`"`) || !strings.Contains(line, `"count":1`) {
		t.Fatal(line)
	}
	if line := call("primary", CmdDecide, map[string]any{"id": id, "decision": "deny", "reason": "no"}); !strings.Contains(line, `"decision":"deny"`) {
		t.Fatal(line)
	}
	if out := <-outcome; out.Decision != approval.DecisionDeny || out.Reason != "no" || out.ResolvedBy != "operator" {
		t.Fatal("the waiting request is denied by the operator", out)
	}
	if line := call("primary", CmdDecide, map[string]any{"id": id, "decision": "grant"}); !strings.Contains(line, approval.ErrUnknownApproval.Error()) {
		t.Fatal(line)
	}
}
