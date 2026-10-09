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

func TestTenantNativeTypedRegistry(t *testing.T) {
	for cmd, want := range map[string]struct{ readOnly, routed bool }{
		CmdTenantCreate: {}, CmdTenantRelease: {}, CmdTenantRemove: {}, CmdTenantToken: {},
		CmdTenantList: {readOnly: true}, CmdTenantStats: {readOnly: true, routed: true},
	} {
		found := 0
		for _, operation := range registeredAppOperations() {
			spec := operation.Spec()
			if spec.Name != cmd {
				continue
			}
			found++
			if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || spec.ReadOnly != want.readOnly || !spec.AllowUnknownInput || spec.Authz != opapi.PrimaryOnly || (spec.Tenancy == opapi.CallerTenant) != want.routed {
				t.Fatalf("%s metadata=%+v", cmd, spec)
			}
		}
		wire, exists := commandRegistry[cmd]
		if found != 1 || !exists || !wire.AppOwned || wire.ReadOnly != want.readOnly || wire.TenantAllowed || wire.TenantRouted != want.routed || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire found=%d metadata=%+v", cmd, found, wire)
		}
	}
}

// A daemon without a tenant registry refuses every tenant operation with the
// same error, before reading any argument.
func TestTenantNativeDisabled(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, dir)
	s.token = "primary"
	for _, cmd := range []string{CmdTenantCreate, CmdTenantList, CmdTenantRelease, CmdTenantRemove, CmdTenantToken, CmdTenantStats} {
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(3 * time.Second))
		done := make(chan struct{})
		go func() { defer close(done); s.handleConn(context.Background(), b) }()
		raw, _ := json.Marshal(Request{ID: "t", Cmd: cmd, Token: "primary", Args: map[string]any{"id": 3}})
		if _, err := a.Write(append(raw, 10)); err != nil {
			t.Fatal(err)
		}
		line, err := bufio.NewReader(a).ReadBytes(10)
		a.Close()
		<-done
		if err != nil || !strings.Contains(string(line), `"error":"multi-tenancy is disabled (no tenant registry configured)"`) {
			t.Fatal(cmd, string(line), err)
		}
	}
}

// The activity summary reads each tenant's own runs: spend, outcomes and the
// latest activity, and a closed tenant is released again afterwards.
func TestTenantNativeStats(t *testing.T) {
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
	for _, e := range []struct {
		kind    event.Kind
		corr    string
		payload map[string]any
	}{
		{event.KindTaskReceived, "run-a", map[string]any{"intent": "a"}},
		{event.KindBudgetConsumed, "run-a", map[string]any{"cost_microcents": float64(700), "model": "m"}},
		{event.KindTaskFailed, "run-a", map[string]any{"reason": "error"}},
		{event.KindTaskReceived, "run-b", map[string]any{"intent": "b"}},
		{event.KindBudgetConsumed, "run-b", map[string]any{"cost_microcents": float64(50), "model": "m"}},
		{event.KindTaskCompleted, "run-b", map[string]any{"iters": float64(1)}},
	} {
		if _, err := tk.Bus().Publish(event.Spec{Subject: "agent.task", Kind: e.kind, Actor: "agent", CorrelationID: e.corr, Payload: e.payload}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := reg.Acquire("idle", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Release("idle"); err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, dir)
	s.token = "primary"
	s.SetTenants(reg)
	a, b := net.Pipe()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(context.Background(), b) }()
	raw, _ := json.Marshal(Request{ID: "t", Cmd: CmdTenantStats, Token: "primary"})
	if _, err := a.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(a).ReadBytes(10)
	a.Close()
	<-done
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"id":"acme"`, `"runs":2`, `"completed":1`, `"failed":1`, `"spent_microcents":750`, `"total_spent_microcents":750`, `"id":"idle"`} {
		if !strings.Contains(string(line), want) {
			t.Fatal(want, string(line))
		}
	}
	infos, _ := reg.List()
	for _, i := range infos {
		if i.ID == "idle" && i.Open {
			t.Fatal("a closed tenant is released again after the summary")
		}
	}
}
