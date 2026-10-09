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
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestReaperNativeTypedRegistry(t *testing.T) {
	found := 0
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if spec.Name != CmdReaperScan {
			continue
		}
		found++
		if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || !spec.ReadOnly || !spec.AllowUnknownInput || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
			t.Fatalf("metadata=%+v", spec)
		}
	}
	wire, exists := commandRegistry[CmdReaperScan]
	if found != 1 || !exists || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
		t.Fatalf("native wire found=%d metadata=%+v", found, wire)
	}
}

// The scan judges the primary roster on the daemon clock whatever tenant is
// named; a tenant token is refused.
func TestReaperNativeScan(t *testing.T) {
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
	tenantToken, _ := reg.Token("acme")
	k.Roster().SetNowForTest(func() time.Time { return time.Now().Add(-10 * 24 * time.Hour) })
	if _, err := k.Roster().Add(roster.Profile{Slug: "dormant"}); err != nil {
		t.Fatal(err)
	}
	tk := entry.Kernel.(*runtime.Kernel)
	tk.Roster().SetNowForTest(func() time.Time { return time.Now().Add(-10 * 24 * time.Hour) })
	if _, err := tk.Roster().Add(roster.Profile{Slug: "tenant-dormant"}); err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, dir)
	s.token = "primary"
	s.SetTenants(reg)
	call := func(token string, args map[string]any) string {
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(3 * time.Second))
		done := make(chan struct{})
		go func() { defer close(done); s.handleConn(context.Background(), b) }()
		raw, _ := json.Marshal(Request{ID: "r", Cmd: CmdReaperScan, Token: token, Args: args})
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
	if line := call("primary", map[string]any{"tenant": "acme"}); !strings.Contains(line, `"dead_count":0`) || !strings.Contains(line, `"idle_days":30`) {
		t.Fatal("a ten-day-old agent is within the default thirty-day grace", line)
	}
	line := call("primary", map[string]any{"idle_days": 7, "tenant": "acme"})
	if !strings.Contains(line, `"dead_agents":[{"last_active_ms":0,"name":"dormant","slug":"dormant"}]`) || strings.Contains(line, "tenant-dormant") || !strings.Contains(line, `"routing_unstable_agents":[]`) {
		t.Fatal("a seven-day window judges the primary roster only, and empty findings are lists", line)
	}
	if line := call(tenantToken, map[string]any{"tenant": "acme"}); !strings.Contains(line, "forbidden") {
		t.Fatal(line)
	}
}
