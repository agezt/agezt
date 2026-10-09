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

func TestJournalNativeTypedRegistry(t *testing.T) {
	for cmd, routed := range map[string]bool{CmdJournalHead: false, CmdJournalTail: false, CmdJournalGrep: false, CmdJournalExport: false, CmdJournalStats: true, CmdChangelog: true, CmdCacheStats: true} {
		tenantAllowed := cmd == CmdCacheStats
		found := 0
		for _, operation := range registeredAppOperations() {
			spec := operation.Spec()
			if spec.Name != cmd {
				continue
			}
			found++
			if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || !spec.ReadOnly || !spec.AllowUnknownInput || (spec.Authz == opapi.OwnTenant) != tenantAllowed || (spec.Tenancy == opapi.CallerTenant) != routed {
				t.Fatalf("%s metadata=%+v", cmd, spec)
			}
		}
		wire, exists := commandRegistry[cmd]
		if found != 1 || !exists || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed != tenantAllowed || wire.TenantRouted != routed || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire found=%d metadata=%+v", cmd, found, wire)
		}
	}
}

// Head, tail, grep and export always read the primary journal; stats follows
// an operator-named tenant; a tenant token reaches none of them.
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
	if _, err := k.Bus().Publish(event.Spec{Subject: "policy", Kind: event.KindPolicyChanged, Actor: "test", Payload: map[string]any{"rule": "deny shell"}}); err != nil {
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
	grep, line := call("primary", CmdJournalGrep, map[string]any{"tenant": "acme", "kind": "task.received"})
	if events, _ := grep["events"].([]any); len(events) != 1 || grep["head"] != float64(pHead) || !strings.Contains(line, `"events":[{"id":"`) {
		t.Fatal("grep reads the primary journal and keeps the event member order", line)
	}
	export, line := call("primary", CmdJournalExport, map[string]any{"tenant": "acme"})
	if export["count"] != float64(pHead+1) || export["head_seq"] != float64(pHead) || export["head_hash"] != pHash || !strings.Contains(line, `"events":[{"id":"`) {
		t.Fatal("export bundles the primary journal and keeps the event member order", line)
	}
	if recent, line := call("primary", CmdJournalExport, map[string]any{"since_ms": 1}); recent["count"] != float64(0) || recent["first_seq"] != float64(-1) {
		t.Fatal("the export window is measured on the daemon clock", line)
	}
	if MaxJournalExportN() != 200_000 {
		t.Fatal("the CLI names the app export cap", MaxJournalExportN())
	}
	if log, line := call("primary", CmdChangelog, map[string]any{"tenant": "acme"}); log["count"] != float64(0) || !strings.Contains(line, `"entries":[]`) {
		t.Fatal("the changelog follows the operator-named tenant", line)
	}
	if log, line := call("primary", CmdChangelog, nil); log["count"] != float64(1) || !strings.Contains(line, `"detail":"deny shell"`) {
		t.Fatal("the primary changelog holds the policy change", line)
	}
	if cache, line := call(tenantToken, CmdCacheStats, acme); cache["calls"] != float64(0) || !strings.Contains(line, `"window_ms":0`) {
		t.Fatal("a tenant reads its own cache statistics", line)
	}
	for _, cmd := range []string{CmdJournalHead, CmdJournalTail, CmdJournalGrep, CmdJournalExport, CmdJournalStats, CmdChangelog} {
		if _, line := call(tenantToken, cmd, acme); !strings.Contains(line, "forbidden") {
			t.Fatal(cmd, line)
		}
	}
}
