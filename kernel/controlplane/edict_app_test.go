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
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestEdictReadsNativeTypedRegistry(t *testing.T) {
	for _, cmd := range []string{CmdEdictShow, CmdEdictDenyList, CmdEdictTest} {
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

// Each token inspects the policy engine of the kernel it is routed to.
func TestEdictReadsNativeRouting(t *testing.T) {
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
	rules, err := edict.ParseDenyRules("shell:curl evil.example")
	if err != nil || len(rules) != 1 {
		t.Fatal(rules, err)
	}
	if _, err := tk.Edict().AddHardDeny(rules[0]); err != nil {
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
	acme := map[string]any{"tenant": "acme"}
	for _, c := range []struct {
		token, cmd string
		args       map[string]any
		has        bool
	}{
		{"primary", CmdEdictShow, nil, false},
		{"primary", CmdEdictDenyList, nil, false},
		{"primary", CmdEdictShow, map[string]any{"tenant": " acme "}, true},
		{tenantToken, CmdEdictShow, acme, true},
		{tenantToken, CmdEdictDenyList, acme, true},
	} {
		if line := call(c.token, c.cmd, c.args); strings.Contains(line, `"substring":"curl evil.example"`) != c.has || !strings.Contains(line, `"type":"result"`) {
			t.Fatalf("%s %v: %s", c.cmd, c.args, line)
		}
	}
	probe := map[string]any{"tenant": "acme", "capability": "shell", "input": "curl evil.example"}
	if line := call(tenantToken, CmdEdictTest, probe); !strings.Contains(line, `"hard_denied":true`) {
		t.Fatal(line)
	}
	if line := call("primary", CmdEdictTest, map[string]any{"capability": "shell", "input": "curl evil.example"}); !strings.Contains(line, `"hard_denied":false`) {
		t.Fatal(line)
	}
	if line := call("primary", CmdEdictShow, map[string]any{"tenant": 3}); !strings.Contains(line, `"error":"args.tenant must be a string"`) {
		t.Fatal(line)
	}
}

func TestEdictWritesNativeTypedRegistry(t *testing.T) {
	for _, cmd := range []string{CmdEdictDenyAdd, CmdEdictDenyRemove, CmdEdictSetLevel, CmdEdictSetMode} {
		found := 0
		for _, operation := range registeredAppOperations() {
			spec := operation.Spec()
			if spec.Name != cmd {
				continue
			}
			found++
			if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || spec.ReadOnly || !spec.AllowUnknownInput || spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant || spec.HTTP.Method != "POST" {
				t.Fatalf("%s metadata=%+v", cmd, spec)
			}
		}
		wire, exists := commandRegistry[cmd]
		if found != 1 || !exists || !wire.AppOwned || wire.ReadOnly || !wire.TenantAllowed || !wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire found=%d metadata=%+v", cmd, found, wire)
		}
	}
}

// Each token changes the policy engine of the kernel it is routed to and
// journals the change there, after the operation's audit record.
func TestEdictWritesNativeRouting(t *testing.T) {
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
	countRules := func(kk *runtime.Kernel) int { return len(kk.Edict().HardDenyRules()) }
	primaryRules, tenantRules := countRules(k), countRules(tk)
	if line := call(tenantToken, CmdEdictDenyAdd, map[string]any{"tenant": "acme", "rule": "shell:curl evil.example"}); !strings.Contains(line, `"substring":"curl evil.example"`) {
		t.Fatal(line)
	}
	if countRules(tk) != tenantRules+1 || countRules(k) != primaryRules {
		t.Fatal("the tenant rule lands on the tenant engine only")
	}
	mode := edict.AskPolicy(1).String()
	if line := call("primary", CmdEdictSetMode, map[string]any{"tenant": " acme ", "mode": mode}); !strings.Contains(line, `"to":"`+mode+`"`) || tk.Edict().AskPolicy().String() != mode {
		t.Fatal(line)
	}
	if line := call("primary", CmdEdictSetMode, map[string]any{"mode": mode, "tenant": 3}); !strings.Contains(line, `"error":"args.tenant must be a string"`) {
		t.Fatal(line)
	}
	events, err := tk.Journal().Tail(1000)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, ev := range events {
		if ev.Kind == "policy.changed" || strings.HasPrefix(string(ev.Kind), "op.") {
			kinds = append(kinds, string(ev.Kind))
		}
	}
	if strings.Join(kinds, " ") != "op.invoked policy.changed op.completed op.invoked policy.changed op.completed" {
		t.Fatal("the tenant journal carries each audit record around its policy change", kinds)
	}
}
