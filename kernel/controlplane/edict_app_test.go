// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
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

func TestEdictDecisionsNativeTypedRegistry(t *testing.T) {
	for _, cmd := range []string{CmdEdictLog, CmdEdictStats} {
		found := 0
		for _, operation := range registeredAppOperations() {
			spec := operation.Spec()
			if spec.Name != cmd {
				continue
			}
			found++
			if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || !spec.ReadOnly || !spec.AllowUnknownInput || spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant || spec.HTTP.Method != "GET" {
				t.Fatalf("%s metadata=%+v", cmd, spec)
			}
		}
		wire, exists := commandRegistry[cmd]
		if found != 1 || !exists || !wire.AppOwned || !wire.ReadOnly || !wire.TenantAllowed || !wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire found=%d metadata=%+v", cmd, found, wire)
		}
	}
}

// Each token reads the policy decisions journaled by the kernel it is routed
// to, on the daemon clock.
func TestEdictDecisionsNativeRouting(t *testing.T) {
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
	decide := func(kk *runtime.Kernel, tool string, allow bool) {
		if _, err := kk.Bus().Publish(event.Spec{Subject: "agent.policy", Kind: event.KindPolicyDecision, Actor: "agent", CorrelationID: "c-" + tool, Payload: map[string]any{"tool": tool, "capability": "shell", "allow": allow}}); err != nil {
			t.Fatal(err)
		}
	}
	decide(k, "primary-tool", true)
	decide(tk, "tenant-tool", false)
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
		{"primary", CmdEdictLog, nil, `"tool":"primary-tool"`},
		{"primary", CmdEdictLog, map[string]any{"tenant": " acme "}, `"tool":"tenant-tool"`},
		{tenantToken, CmdEdictLog, map[string]any{"tenant": "acme", "denied": true}, `"tool":"tenant-tool"`},
		{"primary", CmdEdictStats, nil, `"allowed":1`},
		{tenantToken, CmdEdictStats, map[string]any{"tenant": "acme", "since_ms": 600000}, `"denied":1`},
		{"primary", CmdEdictLog, map[string]any{"denied": "yes"}, `"error":"args.denied must be a boolean"`},
	} {
		if line := call(c.token, c.cmd, c.args); !strings.Contains(line, c.want) {
			t.Fatalf("%s %v: %s", c.cmd, c.args, line)
		}
	}
	if line := call(tenantToken, CmdEdictLog, map[string]any{"tenant": "acme"}); strings.Contains(line, "primary-tool") {
		t.Fatal("a tenant reads only its own decisions", line)
	}
	// A 1 ms window excludes every decision only once the clock has moved past
	// the newest one.
	newest, err := k.Journal().Tail(1)
	if err != nil || len(newest) != 1 {
		t.Fatal(newest, err)
	}
	for time.Now().UnixMilli() <= newest[0].TSUnixMS+1 {
		time.Sleep(time.Millisecond)
	}
	if line := call("primary", CmdEdictStats, map[string]any{"since_ms": 1}); !strings.Contains(line, `"total":0`) || !strings.Contains(line, `"window_ms":1`) {
		t.Fatal("the window follows the daemon clock", line)
	}
}

func TestEdictOverlayNativeTypedRegistry(t *testing.T) {
	for cmd, tenantAllowed := range map[string]bool{CmdEdictOverlay: true, CmdEdictCompact: false} {
		found := 0
		for _, operation := range registeredAppOperations() {
			spec := operation.Spec()
			if spec.Name != cmd {
				continue
			}
			found++
			if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || spec.ReadOnly || !spec.AllowUnknownInput || (spec.Authz == opapi.OwnTenant) != tenantAllowed || spec.Tenancy != opapi.CallerTenant || spec.HTTP != (opapi.HTTP{}) {
				t.Fatalf("%s metadata=%+v", cmd, spec)
			}
		}
		wire, exists := commandRegistry[cmd]
		if found != 1 || !exists || !wire.AppOwned || wire.ReadOnly || wire.TenantAllowed != tenantAllowed || !wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire found=%d metadata=%+v", cmd, found, wire)
		}
	}
}

// The overlay folds the routed kernel's history; compaction writes that
// kernel's snapshot and journals its hash there, and only the primary may run it.
func TestEdictOverlayNativeRouting(t *testing.T) {
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
	if line := call(tenantToken, CmdEdictDenyAdd, map[string]any{"tenant": "acme", "rule": "shell:tenant-only"}); !strings.Contains(line, `"type":"result"`) {
		t.Fatal(line)
	}
	if line := call("primary", CmdEdictOverlay, nil); !strings.Contains(line, `"empty":true`) {
		t.Fatal("the primary overlay is untouched", line)
	}
	if line := call(tenantToken, CmdEdictOverlay, map[string]any{"tenant": "acme"}); !strings.Contains(line, `"substring":"tenant-only"`) || !strings.Contains(line, `"changes_folded":1`) {
		t.Fatal(line)
	}
	if line := call(tenantToken, CmdEdictCompact, map[string]any{"tenant": "acme"}); !strings.Contains(line, `"type":"error"`) {
		t.Fatal("a tenant token may not compact", line)
	}
	snapshot := func(kk *runtime.Kernel) string {
		raw, _ := os.ReadFile(filepath.Join(kk.BaseDir(), "runtime", edict.OverlaySnapshotFile))
		return string(raw)
	}
	if line := call("primary", CmdEdictCompact, map[string]any{"tenant": " acme "}); !strings.Contains(line, `"folded":1`) || !strings.Contains(line, `"compacted":1`) || !strings.Contains(snapshot(tk), "tenant-only") || snapshot(k) != "" {
		t.Fatal("an operator-named compaction writes the tenant snapshot only", line)
	}
	events, err := tk.Journal().Tail(1)
	if err != nil || len(events) != 1 || events[0].Kind != event.KindOpCompleted {
		t.Fatal(events, err)
	}
	compacted := 0
	_ = tk.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindPolicyCompacted {
			compacted++
		}
		return nil
	})
	if compacted != 1 {
		t.Fatal("the tenant journal vouches for its snapshot", compacted)
	}
}
