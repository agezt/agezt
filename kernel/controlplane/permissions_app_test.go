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
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestAgentPermissionsNativeTypedRegistry(t *testing.T) {
	for cmd, readOnly := range map[string]bool{CmdAgentPermissions: true, CmdAgentCapabilities: false} {
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

// The permission picture and the capability patch act on the primary roster;
// a patch is audited, a tenant token reaches neither.
func TestAgentPermissionsNativeRouting(t *testing.T) {
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
	if _, err := k.AddProfile(roster.Profile{Slug: "builder"}); err != nil {
		t.Fatal(err)
	}
	if _, err := tk.AddProfile(roster.Profile{Slug: "builder", TrustCeiling: "L1"}); err != nil {
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
		raw, _ := json.Marshal(Request{ID: "p", Cmd: cmd, Token: token, Args: args})
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
	if line := call("primary", CmdAgentPermissions, map[string]any{"ref": "builder", "tenant": "acme"}); !strings.Contains(line, `"trust_ceiling":""`) || !strings.Contains(line, `"config_entries":[]`) {
		t.Fatal("the picture reads the primary roster whatever tenant is named", line)
	}
	head, _ := k.Journal().Head()
	if line := call("primary", CmdAgentCapabilities, map[string]any{"ref": "builder", "trust_ceiling": "L2", "tenant": "acme"}); !strings.Contains(line, `"profile":{`) || !strings.Contains(line, `"trust_ceiling":"L2"`) {
		t.Fatal(line)
	}
	if p, _ := k.Roster().Get("builder"); p.TrustCeiling != "L2" {
		t.Fatal("the patch lands on the primary roster", p)
	}
	if p, _ := tk.Roster().Get("builder"); p.TrustCeiling != "L1" {
		t.Fatal("the tenant roster is untouched", p)
	}
	audited := map[event.Kind]bool{}
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Seq > head {
			audited[e.Kind] = true
		}
		return nil
	})
	if !audited[event.KindOpInvoked] {
		t.Fatal("the capability patch is audited", audited)
	}
	if line := call("primary", CmdAgentCapabilities, map[string]any{"ref": "builder"}); !strings.Contains(line, `"error":"args capability field required"`) {
		t.Fatal(line)
	}
	for _, cmd := range []string{CmdAgentPermissions, CmdAgentCapabilities} {
		if line := call(tenantToken, cmd, map[string]any{"tenant": "acme", "ref": "builder", "workdir": "x"}); !strings.Contains(line, "forbidden") {
			t.Fatal(cmd, line)
		}
	}
}
