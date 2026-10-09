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
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestStateNativeTypedRegistry(t *testing.T) {
	for _, cmd := range []string{CmdStateList, CmdStateGet} {
		found := 0
		for _, operation := range registeredAppOperations() {
			spec := operation.Spec()
			if spec.Name != cmd {
				continue
			}
			found++
			if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || !spec.ReadOnly || !spec.AllowUnknownInput || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
				t.Fatalf("%s metadata=%+v", cmd, spec)
			}
		}
		wire, exists := commandRegistry[cmd]
		if found != 1 || !exists || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire found=%d metadata=%+v", cmd, found, wire)
		}
	}
}

// Both reads inspect the primary kernel's state store whatever tenant is
// named; a tenant token reaches neither.
func TestStateNativeRouting(t *testing.T) {
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
	if err := k.State().Set("primary_ns", "k", map[string]any{"n": 1}); err != nil {
		t.Fatal(err)
	}
	if err := entry.Kernel.(*runtime.Kernel).State().Set("tenant_ns", "k", "acme"); err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, dir)
	s.token = "primary"
	s.SetTenants(reg)
	call := func(token, cmd string, args map[string]any) string {
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(3 * time.Second))
		done := make(chan struct{})
		go func() { defer close(done); s.handleConn(context.Background(), b) }()
		raw, _ := json.Marshal(Request{ID: "s", Cmd: cmd, Token: token, Args: args})
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
	if line := call("primary", CmdStateList, map[string]any{"tenant": "acme"}); !strings.Contains(line, `"namespaces":["primary_ns"]`) {
		t.Fatal("the listing reads the primary store", line)
	}
	if line := call("primary", CmdStateGet, map[string]any{"tenant": "acme", "namespace": "primary_ns", "key": "k"}); !strings.Contains(line, `"value":{"n":1}`) {
		t.Fatal(line)
	}
	if line := call("primary", CmdStateGet, map[string]any{"namespace": "tenant_ns", "key": "k", "tenant": "acme"}); !strings.Contains(line, `"found":false`) {
		t.Fatal("a tenant's store is never read", line)
	}
	if line := call("primary", CmdStateList, map[string]any{"namespace": "../x"}); !strings.Contains(line, `"type":"error"`) {
		t.Fatal(line)
	}
	for _, cmd := range []string{CmdStateList, CmdStateGet} {
		if line := call(tenantToken, cmd, map[string]any{"tenant": "acme", "namespace": "tenant_ns", "key": "k"}); !strings.Contains(line, "forbidden") {
			t.Fatal(cmd, line)
		}
	}
}
