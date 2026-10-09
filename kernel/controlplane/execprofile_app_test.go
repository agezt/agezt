// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

// TestExecutionProfilesReadTheCallersKernel: the primary kernel carries the
// shell tool and the tenant's kernel none, so each read shows whose tools built
// the inventory.
func TestExecutionProfilesReadTheCallersKernel(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), Tools: map[string]toolapi.Tool{"shell": shell.NewWithWarden(warden.New(nil))}})
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
	if _, err := reg.Acquire("acme", time.Now()); err != nil {
		t.Fatal(err)
	}
	tenantToken, _ := reg.Token("acme")
	s := NewServer(k, dir)
	s.token = "primary"
	s.SetTenants(reg)
	read := func(token, cmd string, args map[string]any) string {
		t.Helper()
		resp := callAppHost(t, s, Request{ID: "x", Cmd: cmd, Token: token, Args: args})[0]
		if resp.Type != RespResult {
			t.Fatal(cmd, args, resp.Error)
		}
		raw, err := json.Marshal(resp.Result)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	local := func(raw string) bool { return strings.Contains(raw, `"tools":["shell"]`) }
	for _, cmd := range []string{CmdExecutionProfiles, CmdExecutionProfileShow} {
		args := map[string]any{}
		if cmd == CmdExecutionProfileShow {
			args["id"] = " local "
		}
		if !local(read("primary", cmd, args)) {
			t.Fatal(cmd, "the primary reads its own kernel")
		}
		args["tenant"] = "acme"
		if local(read("primary", cmd, args)) || local(read(tenantToken, cmd, args)) {
			t.Fatal(cmd, "a named tenant reads its own kernel")
		}
	}
	if got := read(tenantToken, CmdExecutionProfileCheck, map[string]any{"tenant": "acme"}); !strings.Contains(got, `"routable_run_profiles":`) || !strings.Contains(got, `"checks":[{`) {
		t.Fatal(got)
	}
	for _, cmd := range []string{CmdExecutionProfiles, CmdExecutionProfileShow, CmdExecutionProfileCheck} {
		if wire, exists := commandRegistry[cmd]; !exists || !wire.AppOwned || !wire.ReadOnly || !wire.TenantAllowed || !wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire %+v", cmd, wire)
		}
	}
}
