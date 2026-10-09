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

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestSteerNativeTypedRegistry(t *testing.T) {
	for _, cmd := range []string{CmdCancelRun, CmdRunPause, CmdRunResume, CmdRunStep, CmdRunSteer, CmdRunIntervene} {
		found := 0
		for _, operation := range registeredAppOperations() {
			spec := operation.Spec()
			if spec.Name != cmd {
				continue
			}
			found++
			if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || spec.ReadOnly || !spec.AllowUnknownInput || spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant {
				t.Fatalf("%s metadata=%+v", cmd, spec)
			}
		}
		wire, exists := commandRegistry[cmd]
		if found != 1 || !exists || !wire.AppOwned || wire.ReadOnly || !wire.TenantAllowed || !wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire found=%d metadata=%+v", cmd, found, wire)
		}
	}
}

// blockedRun starts a run whose model call blocks until release is closed, and
// returns once the run is in flight.
func blockedRun(t *testing.T, k *runtime.Kernel, prov *mock.Provider) (string, chan struct{}, chan struct{}) {
	t.Helper()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	prov.OnRequest = func(llm.CompletionRequest) {
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
	}
	corr := k.NewCorrelation()
	go func() { defer close(done); _, _ = k.RunWith(context.Background(), corr, "work") }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("run never reached the model")
	}
	return corr, release, done
}

// Each token steers the runs of the kernel it is routed to: a tenant steers its
// own, and the primary token reaches a tenant's run only by naming the tenant.
func TestSteerNativeRoutesToTheCallersKernel(t *testing.T) {
	dir := t.TempDir()
	primaryProv := mock.New(mock.FinalText("p"))
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: primaryProv})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	tenantProv := mock.New(mock.FinalText("t"))
	reg, err := tenant.New(filepath.Join(dir, "tenants"), func(_ string, baseDir string) (io.Closer, error) {
		return runtime.Open(runtime.Config{BaseDir: baseDir, Provider: tenantProv})
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
	pCorr, pRelease, pDone := blockedRun(t, k, primaryProv)
	tCorr, tRelease, tDone := blockedRun(t, tk, tenantProv)
	call := func(token, cmd string, args map[string]any) string {
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(3 * time.Second))
		done := make(chan struct{})
		go func() { defer close(done); s.handleConn(context.Background(), b) }()
		raw, _ := json.Marshal(Request{ID: "steer", Cmd: cmd, Token: token, Args: args})
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
		{"primary", CmdRunPause, map[string]any{"correlation": pCorr}, `"ok":true`},
		{"primary", CmdRunPause, map[string]any{"correlation": tCorr}, `"ok":false`},
		{"primary", CmdRunPause, map[string]any{"correlation": tCorr, "tenant": "acme"}, `"ok":true`},
		{tenantToken, CmdRunResume, map[string]any{"correlation": tCorr, "tenant": "acme"}, `"ok":true`},
		{tenantToken, CmdRunStep, map[string]any{"correlation": pCorr, "tenant": "acme"}, `"ok":false`},
		{tenantToken, CmdRunSteer, map[string]any{"correlation": tCorr, "tenant": "acme", "directive": "focus", "mode": "note"}, `"accepted":true,"correlation":"` + tCorr + `","mode":"note"`},
		{"primary", CmdRunIntervene, map[string]any{"correlation": pCorr, "primitive": "halt", "idempotency_key": "k1"}, `"accepted":true`},
		{tenantToken, CmdCancelRun, map[string]any{"correlation": pCorr, "tenant": "acme"}, `"cancelled":false`},
		{"primary", CmdCancelRun, map[string]any{"correlation": tCorr, "tenant": 3}, `"error":"args.tenant must be a string"`},
	} {
		if line := call(c.token, c.cmd, c.args); !strings.Contains(line, c.want) {
			t.Fatalf("%s %v: %s", c.cmd, c.args, line)
		}
	}
	if paused, _, ok := k.RunControlState(pCorr); !ok || !paused {
		t.Fatal("primary run should be paused")
	}
	if paused, pending, ok := tk.RunControlState(tCorr); !ok || paused || pending != 1 {
		t.Fatalf("tenant run paused=%v pending=%d ok=%v", paused, pending, ok)
	}
	k.CancelRun(pCorr)
	tk.CancelRun(tCorr)
	close(pRelease)
	close(tRelease)
	<-pDone
	<-tDone
}
