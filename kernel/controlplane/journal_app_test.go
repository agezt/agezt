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

func TestJournalNativeTypedRegistry(t *testing.T) {
	for cmd, routed := range map[string]bool{CmdJournalHead: false, CmdJournalTail: false, CmdJournalStats: true} {
		found := 0
		for _, operation := range registeredAppOperations() {
			spec := operation.Spec()
			if spec.Name != cmd {
				continue
			}
			found++
			if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || !spec.ReadOnly || !spec.AllowUnknownInput || spec.Authz != opapi.PrimaryOnly || (spec.Tenancy == opapi.CallerTenant) != routed {
				t.Fatalf("%s metadata=%+v", cmd, spec)
			}
		}
		wire, exists := commandRegistry[cmd]
		if found != 1 || !exists || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted != routed || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire found=%d metadata=%+v", cmd, found, wire)
		}
	}
}

// Head and tail always read the primary journal; stats follows an
// operator-named tenant; a tenant token reaches none of them.
func TestJournalNativeRouting(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(mock.FinalText("p"))})
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
	if _, _, err := k.Run(context.Background(), "primary work"); err != nil {
		t.Fatal(err)
	}
	pHead, pHash := k.Journal().Head()
	tHead, _ := tk.Journal().Head()
	s := NewServer(k, dir)
	s.token = "primary"
	s.SetTenants(reg)
	call := func(token, cmd string, args map[string]any) (map[string]any, string) {
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(3 * time.Second))
		done := make(chan struct{})
		go func() { defer close(done); s.handleConn(context.Background(), b) }()
		raw, _ := json.Marshal(Request{ID: "j", Cmd: cmd, Token: token, Args: args})
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
		_ = json.Unmarshal(line, &resp)
		return resp.Result, string(line)
	}
	acme := map[string]any{"tenant": "acme"}
	if head, line := call("primary", CmdJournalHead, acme); head["head"] != float64(pHead) || head["hash"] != pHash {
		t.Fatal("head reads the primary journal", line)
	}
	tail, line := call("primary", CmdJournalTail, map[string]any{"tenant": "acme", "n": 2})
	if events, _ := tail["events"].([]any); len(events) != 2 || tail["head"] != float64(pHead) || !strings.Contains(line, `"events":[{"id":"`) {
		t.Fatal("tail reads the primary journal and keeps the event member order", line)
	}
	if stats, line := call("primary", CmdJournalStats, nil); stats["events"] != float64(pHead+1) || stats["segments"] != float64(1) {
		t.Fatal(line)
	}
	if stats, line := call("primary", CmdJournalStats, map[string]any{"tenant": " acme "}); stats["events"] != float64(tHead+1) {
		t.Fatal("stats follows the operator-named tenant", line, tHead)
	}
	for _, cmd := range []string{CmdJournalHead, CmdJournalTail, CmdJournalStats} {
		if _, line := call(tenantToken, cmd, acme); !strings.Contains(line, "forbidden") {
			t.Fatal(cmd, line)
		}
	}
}
