// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// withTenants builds a tenant registry under dir, wires it into srv, and
// returns it. The OpenFunc opens a real isolated kernel per tenant.
func withTenants(t *testing.T, srv *controlplane.Server, dir string) *tenant.Registry {
	t.Helper()
	reg, err := tenant.New(filepath.Join(dir, "tenants"), func(id, tdir string) (io.Closer, error) {
		return runtime.Open(runtime.Config{
			BaseDir:  tdir,
			Provider: mock.New(mock.FinalText("tenant-ok")),
			Tools:    map[string]agent.Tool{},
		})
	})
	if err != nil {
		t.Fatalf("tenant.New: %v", err)
	}
	// Close tenant kernels before the TempDir cleanup runs, else their open
	// journal files block removal on Windows.
	t.Cleanup(func() { _ = reg.CloseAll() })
	srv.SetTenants(reg)
	return reg
}

func mustTenant(t *testing.T, reg *tenant.Registry, id string) string {
	t.Helper()
	if _, err := reg.Acquire(id, time.Now()); err != nil {
		t.Fatalf("Acquire(%s): %v", id, err)
	}
	tok, err := reg.Token(id)
	if err != nil {
		t.Fatalf("Token(%s): %v", id, err)
	}
	return tok
}

// tenantClient returns a client that authenticates with the given token via
// the AGEZT_TOKEN override (t.Setenv scopes it to this test).
func tenantClient(t *testing.T, dir, token string) *controlplane.Client {
	t.Helper()
	t.Setenv("AGEZT_TOKEN", token)
	c, err := controlplane.NewClient(dir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

// TestTenantToken_AuthorizesOwnTenant — a tenant token authenticates and may
// run an allowlisted, tenant-routed command on its OWN tenant (M38).
func TestTenantToken_AuthorizesOwnTenant(t *testing.T) {
	_, srv, _, dir := startPair(t, mock.New(mock.FinalText("ok")))
	reg := withTenants(t, srv, dir)
	tok := mustTenant(t, reg, "acme")

	c := tenantClient(t, dir, tok)
	if _, err := c.Call(context.Background(), controlplane.CmdEdictShow,
		map[string]any{"tenant": "acme"}); err != nil {
		t.Errorf("tenant token should manage its own edict; got %v", err)
	}
}

// TestTenantToken_RejectsOtherTenant — a tenant token cannot act on a
// DIFFERENT tenant (Authorize fails for the mismatched id).
func TestTenantToken_RejectsOtherTenant(t *testing.T) {
	_, srv, _, dir := startPair(t, mock.New(mock.FinalText("ok")))
	reg := withTenants(t, srv, dir)
	acmeTok := mustTenant(t, reg, "acme")
	_ = mustTenant(t, reg, "beta")

	c := tenantClient(t, dir, acmeTok)
	_, err := c.Call(context.Background(), controlplane.CmdEdictShow,
		map[string]any{"tenant": "beta"})
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("acme token targeting beta should be unauthorized; got %v", err)
	}
}

// TestTenantToken_ForbidsNonAllowlistedCmd — a tenant token is rejected for
// a command outside the tenant allowlist. Tenant-registry management and
// daemon-global commands stay primary-only.
func TestTenantToken_ForbidsNonAllowlistedCmd(t *testing.T) {
	_, srv, _, dir := startPair(t, mock.New(mock.FinalText("ok")))
	reg := withTenants(t, srv, dir)
	tok := mustTenant(t, reg, "acme")

	c := tenantClient(t, dir, tok)
	// Tenant-registry management is primary-only.
	_, err := c.Call(context.Background(), controlplane.CmdTenantList,
		map[string]any{"tenant": "acme"})
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("tenant token running tenant_list should be forbidden; got %v", err)
	}
	// Daemon-global shutdown is primary-only.
	_, err = c.Call(context.Background(), controlplane.CmdHalt,
		map[string]any{"tenant": "acme"})
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("tenant token running halt should be forbidden; got %v", err)
	}
}

// TestTenantToken_AllowsOwnRunStats — a tenant token may read its OWN run
// stats/list (M39 made these tenant-scoped + allowlisted).
func TestTenantToken_AllowsOwnRunStats(t *testing.T) {
	_, srv, _, dir := startPair(t, mock.New(mock.FinalText("ok")))
	reg := withTenants(t, srv, dir)
	tok := mustTenant(t, reg, "acme")

	c := tenantClient(t, dir, tok)
	if _, err := c.Call(context.Background(), controlplane.CmdRunsStats,
		map[string]any{"tenant": "acme"}); err != nil {
		t.Errorf("tenant token should read its own run stats; got %v", err)
	}
	if _, err := c.Call(context.Background(), controlplane.CmdRunsList,
		map[string]any{"tenant": "acme"}); err != nil {
		t.Errorf("tenant token should list its own runs; got %v", err)
	}
}

// TestTenantToken_InvalidRejected — a bogus token never authenticates.
func TestTenantToken_InvalidRejected(t *testing.T) {
	_, srv, _, dir := startPair(t, mock.New(mock.FinalText("ok")))
	reg := withTenants(t, srv, dir)
	_ = mustTenant(t, reg, "acme")

	c := tenantClient(t, dir, "deadbeefnotarealtoken")
	_, err := c.Call(context.Background(), controlplane.CmdEdictShow,
		map[string]any{"tenant": "acme"})
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("bogus token should be unauthorized; got %v", err)
	}
}

// TestRunsAreTenantScoped — runs in a tenant's journal appear only under
// that tenant's scope, never in the primary view, and vice versa (M39).
func TestRunsAreTenantScoped(t *testing.T) {
	k, srv, c, dir := startPair(t, mock.New(mock.FinalText("ok")))
	reg := withTenants(t, srv, dir)
	if _, err := reg.Acquire("acme", time.Now()); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	tk, _ := reg.Get("acme")
	tenantK := tk.Kernel.(*runtime.Kernel)

	publishRun := func(k *runtime.Kernel, corr string) {
		t.Helper()
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
			CorrelationID: corr, Payload: map[string]string{"intent": corr},
		})
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "task", Kind: event.KindTaskCompleted, Actor: "a",
			CorrelationID: corr, Payload: map[string]any{"iters": 1},
		})
	}
	publishRun(k, "primary-run")
	publishRun(tenantK, "tenant-run")

	corrsIn := func(args map[string]any) map[string]bool {
		t.Helper()
		res, err := c.Call(context.Background(), controlplane.CmdRunsList, args)
		if err != nil {
			t.Fatalf("RunsList(%v): %v", args, err)
		}
		rows, _ := res["runs"].([]any)
		seen := map[string]bool{}
		for _, raw := range rows {
			r, _ := raw.(map[string]any)
			if id, _ := r["correlation_id"].(string); id != "" {
				seen[id] = true
			}
		}
		return seen
	}

	// Primary scope: only the primary run.
	prim := corrsIn(nil)
	if !prim["primary-run"] || prim["tenant-run"] {
		t.Errorf("primary view = %v, want only primary-run", prim)
	}
	// Tenant scope: only the tenant run.
	ten := corrsIn(map[string]any{"tenant": "acme"})
	if !ten["tenant-run"] || ten["primary-run"] {
		t.Errorf("tenant view = %v, want only tenant-run", ten)
	}
}

// TestPrimaryToken_RetainsFullAccess — the primary token still authorizes
// everything: a tenant-routed command on any tenant AND tenant-registry
// management (which a tenant token may not do).
func TestPrimaryToken_RetainsFullAccess(t *testing.T) {
	_, srv, c, dir := startPair(t, mock.New(mock.FinalText("ok")))
	reg := withTenants(t, srv, dir)
	_ = mustTenant(t, reg, "acme")

	// Primary managing a tenant's edict.
	if _, err := c.Call(context.Background(), controlplane.CmdEdictShow,
		map[string]any{"tenant": "acme"}); err != nil {
		t.Errorf("primary should manage any tenant's edict; got %v", err)
	}
	// Primary running tenant-registry management.
	if _, err := c.Call(context.Background(), controlplane.CmdTenantList, nil); err != nil {
		t.Errorf("primary should run tenant_list; got %v", err)
	}
}
