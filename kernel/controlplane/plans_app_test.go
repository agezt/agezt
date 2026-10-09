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

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestPlanHistoryNativeTypedRegistry(t *testing.T) {
	for _, cmd := range []string{CmdPlanHistory, CmdPlanStats} {
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

// Each token reads the plan executions journaled by the kernel it is routed to.
func TestPlanHistoryNativeRouting(t *testing.T) {
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
		if _, err := kk.Bus().Publish(event.Spec{Subject: "plan", Kind: kind, Actor: "scheduler", CorrelationID: "plan-1", Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	publish(k, event.KindPlanStarted, map[string]any{"plan_name": "primary-plan", "node_count": 2})
	publish(tk, event.KindPlanStarted, map[string]any{"plan_name": "tenant-plan", "node_count": 3})
	publish(tk, event.KindPlanFailed, map[string]any{"plan_name": "ignored"})
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
		{"primary", CmdPlanHistory, nil, `"plan_name":"primary-plan"`},
		{"primary", CmdPlanHistory, map[string]any{"tenant": " acme "}, `"status":"failed"`},
		{tenantToken, CmdPlanHistory, map[string]any{"tenant": "acme", "status": "failed"}, `"plan_name":"tenant-plan"`},
		{"primary", CmdPlanStats, nil, `"running":1`},
		{tenantToken, CmdPlanStats, map[string]any{"tenant": "acme"}, `"failed":1`},
		{"primary", CmdPlanHistory, map[string]any{"status": 3}, `"error":"args.status must be a string"`},
	} {
		if line := call(c.token, c.cmd, c.args); !strings.Contains(line, c.want) {
			t.Fatalf("%s %v: %s", c.cmd, c.args, line)
		}
	}
	if line := call(tenantToken, CmdPlanHistory, map[string]any{"tenant": "acme"}); strings.Contains(line, "primary-plan") {
		t.Fatal("a tenant reads only its own plans", line)
	}
}
