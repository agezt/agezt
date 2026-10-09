// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestAuditNativeTypedRegistry(t *testing.T) {
	for _, cmd := range []string{CmdNetguardLog, CmdRateLimitLog, CmdRateLimitStats, CmdWardenLog, CmdWardenStats} {
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

// Each token reads the guard records of the kernel it is routed to.
func TestAuditNativeReadsTheCallersKernel(t *testing.T) {
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
	publish := func(kern *runtime.Kernel, kind event.Kind, payload map[string]any) {
		if _, err := kern.Bus().Publish(event.Spec{Subject: "guard", Kind: kind, Actor: "test", Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	publish(k, event.KindNetguardBlocked, map[string]any{"ip": "10.0.0.1", "tool": "http"})
	publish(tk, event.KindNetguardBlocked, map[string]any{"ip": "169.254.169.254", "tool": "browser"})
	publish(tk, event.KindRateLimited, map[string]any{"used": 9, "limit_per_min": 5})
	publish(tk, event.KindWardenExecuted, map[string]any{"profile_effective": "strict", "argv0": "python", "downgraded": true})
	s := NewServer(k, dir)
	s.token = "primary"
	s.SetTenants(reg)
	call := func(token, cmd string, args map[string]any) map[string]any {
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(3 * time.Second))
		done := make(chan struct{})
		go func() { defer close(done); s.handleConn(context.Background(), b) }()
		raw, _ := json.Marshal(Request{ID: "a", Cmd: cmd, Token: token, Args: args})
		if _, err := a.Write(append(raw, 10)); err != nil {
			t.Fatal(err)
		}
		line, err := bufio.NewReader(a).ReadBytes(10)
		a.Close()
		<-done
		if err != nil {
			t.Fatal(err)
		}
		var resp Response
		if err := json.Unmarshal(line, &resp); err != nil || resp.Type != RespResult {
			t.Fatalf("%s %v: %s", cmd, args, line)
		}
		return resp.Result
	}
	ip := func(out map[string]any) any {
		rows, _ := out["blocks"].([]any)
		if len(rows) != 1 {
			t.Fatalf("blocks %v", out)
		}
		return rows[0].(map[string]any)["ip"]
	}
	if got := ip(call("primary", CmdNetguardLog, nil)); got != "10.0.0.1" {
		t.Fatal(got)
	}
	if got := ip(call("primary", CmdNetguardLog, map[string]any{"tenant": " acme "})); got != "169.254.169.254" {
		t.Fatal(got)
	}
	if got := ip(call(tenantToken, CmdNetguardLog, map[string]any{"tenant": "acme"})); got != "169.254.169.254" {
		t.Fatal(got)
	}
	acme := map[string]any{"tenant": "acme"}
	if stats := call(tenantToken, CmdRateLimitStats, acme); stats["throttled"] != float64(1) || stats["worst_used"] != float64(9) {
		t.Fatal(stats)
	}
	if stats := call("primary", CmdRateLimitStats, nil); stats["throttled"] != float64(0) {
		t.Fatal(stats)
	}
	if log := call(tenantToken, CmdRateLimitLog, acme); log["count"] != float64(1) {
		t.Fatal(log)
	}
	if stats := call(tenantToken, CmdWardenStats, acme); stats["executions"] != float64(1) || stats["downgrade_rate"] != float64(1) {
		t.Fatal(stats)
	}
	if log := call(tenantToken, CmdWardenLog, map[string]any{"tenant": "acme", "issues": true}); log["count"] != float64(0) {
		t.Fatal(log)
	}
}
