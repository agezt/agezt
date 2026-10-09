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

	appruns "github.com/agezt/agezt/kernel/app/runs"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestRunsNativeTypedRegistry(t *testing.T) {
	for _, cmd := range []string{CmdRunsList, CmdRunsStats} {
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

// Each token reads the runs of the kernel it is routed to.
func TestRunsNativeReadsTheCallersKernel(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(mock.FinalText("p"))})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	reg, err := tenant.New(filepath.Join(dir, "tenants"), func(_ string, baseDir string) (io.Closer, error) {
		return runtime.Open(runtime.Config{BaseDir: baseDir, Provider: mock.New(mock.FinalText("t"), mock.FinalText("t"))})
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
	if _, _, err := k.Run(context.Background(), "primary work"); err != nil {
		t.Fatal(err)
	}
	for _, intent := range []string{"tenant work", "tenant chore"} {
		if _, _, err := tk.Run(context.Background(), intent); err != nil {
			t.Fatal(err)
		}
	}
	s := NewServer(k, dir)
	s.token = "primary"
	s.SetTenants(reg)
	call := func(token, cmd string, args map[string]any) map[string]any {
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(3 * time.Second))
		done := make(chan struct{})
		go func() { defer close(done); s.handleConn(context.Background(), b) }()
		raw, _ := json.Marshal(Request{ID: "runs", Cmd: cmd, Token: token, Args: args})
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
	for _, c := range []struct {
		token  string
		args   map[string]any
		count  float64
		intent string
	}{
		{"primary", map[string]any{}, 1, "primary work"},
		{"primary", map[string]any{"tenant": " acme "}, 2, "tenant chore"},
		{tenantToken, map[string]any{"tenant": "acme"}, 2, "tenant chore"},
		{tenantToken, map[string]any{"tenant": "acme", "intent": "WORK"}, 1, "tenant work"},
	} {
		list := call(c.token, CmdRunsList, c.args)
		rows, _ := list["runs"].([]any)
		if list["count"] != c.count || len(rows) != int(c.count) || rows[0].(map[string]any)["intent"] != c.intent || rows[0].(map[string]any)["status"] != "completed" {
			t.Fatalf("list %v: %v", c.args, list)
		}
		if stats := call(c.token, CmdRunsStats, c.args); stats["total"] != c.count || stats["completed"] != c.count || stats["success_rate"] != 1.0 {
			t.Fatalf("stats %v: %v", c.args, stats)
		}
	}
}

// Runs that started in the same millisecond keep journal order, newest first:
// the native fold hands the start seq through to the app sort.
func TestRunsNativeSameMillisecondOrder(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.Open(filepath.Join(dir, "journal"), journal.Options{Now: func() time.Time { return time.UnixMilli(1_700_000_000_000) }})
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, corr := range []string{"r1", "r2", "r3", "r4", "r5", "r6", "r7", "r8"} {
		if _, err := j.Append(event.Spec{Subject: "agent.x.task", Kind: event.KindTaskReceived, Actor: "test", CorrelationID: corr, Payload: map[string]any{"intent": corr}}); err != nil {
			t.Fatal(err)
		}
		want = append([]string{corr}, want...)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	out, err := (&Server{}).runReads(k).List(context.Background(), appruns.ListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range out.Runs {
		got = append(got, r.CorrelationID)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("same-millisecond order %v want %v", got, want)
	}
}
