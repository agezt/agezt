// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"io"
	"os"
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

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

func TestTenantCreateListReleaseRemove(t *testing.T) {
	_, srv, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	reg, err := tenant.New(t.TempDir(), func(id, baseDir string) (io.Closer, error) {
		return nopCloser{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.SetTenants(reg)
	ctx := context.Background()

	// Create two tenants.
	res, err := c.Call(ctx, controlplane.CmdTenantCreate, map[string]any{"id": "alpha"})
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	if res["created"] != true || res["id"] != "alpha" {
		t.Errorf("create result = %v", res)
	}
	// Create returns the tenant's per-tenant token, and tenant_token reveals the
	// same value (stable across calls).
	alphaTok, _ := res["token"].(string)
	if len(alphaTok) < 32 {
		t.Errorf("create alpha token looks unminted: %q", alphaTok)
	}
	res, err = c.Call(ctx, controlplane.CmdTenantToken, map[string]any{"id": "alpha"})
	if err != nil {
		t.Fatalf("token alpha: %v", err)
	}
	if res["token"] != alphaTok {
		t.Errorf("tenant_token = %v, want stable %q", res["token"], alphaTok)
	}
	if _, err := c.Call(ctx, controlplane.CmdTenantCreate, map[string]any{"id": "beta"}); err != nil {
		t.Fatalf("create beta: %v", err)
	}

	// Creating an existing one reports created=false (idempotent open).
	res, _ = c.Call(ctx, controlplane.CmdTenantCreate, map[string]any{"id": "alpha"})
	if res["created"] != false {
		t.Errorf("re-create alpha created = %v, want false", res["created"])
	}

	// List → both, open.
	res, _ = c.Call(ctx, controlplane.CmdTenantList, nil)
	if cnt, _ := res["count"].(float64); int(cnt) != 2 {
		t.Errorf("count = %v, want 2", res["count"])
	}

	// Release alpha → closed but still on disk.
	res, _ = c.Call(ctx, controlplane.CmdTenantRelease, map[string]any{"id": "alpha"})
	if res["released"] != true {
		t.Errorf("release alpha = %v", res)
	}
	res, _ = c.Call(ctx, controlplane.CmdTenantList, nil)
	list, _ := res["tenants"].([]any)
	for _, item := range list {
		m, _ := item.(map[string]any)
		if m["id"] == "alpha" && m["open"] != false {
			t.Error("alpha should be closed after release")
		}
		if m["id"] == "beta" && m["open"] != true {
			t.Error("beta should still be open")
		}
	}

	// Remove beta → gone from the listing.
	res, _ = c.Call(ctx, controlplane.CmdTenantRemove, map[string]any{"id": "beta"})
	if res["removed"] != true {
		t.Errorf("remove beta = %v", res)
	}
	res, _ = c.Call(ctx, controlplane.CmdTenantList, nil)
	if cnt, _ := res["count"].(float64); int(cnt) != 1 {
		t.Errorf("count after rm = %v, want 1", res["count"])
	}

	// Invalid id rejected.
	if _, err := c.Call(ctx, controlplane.CmdTenantCreate, map[string]any{"id": "../evil"}); err == nil {
		t.Error("traversal id should be rejected")
	}
}

func TestRun_RoutesToTenantKernel(t *testing.T) {
	// Primary kernel (from startPair) plus a registry of real tenant kernels.
	_, srv, c, primaryDir := startPair(t, mock.New(mock.FinalText("primary")))
	regRoot := t.TempDir()
	reg, err := tenant.New(regRoot, func(id, baseDir string) (io.Closer, error) {
		return runtime.Open(runtime.Config{
			BaseDir:  baseDir,
			Provider: mock.New(mock.FinalText("tenant-" + id)),
			Tools:    map[string]agent.Tool{},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.CloseAll() })
	srv.SetTenants(reg)

	ctx := context.Background()
	// Route a run to tenant "alpha" (CmdRun streams events, so use Stream).
	res, err := c.Stream(ctx, controlplane.CmdRun, map[string]any{
		"intent": "routed-to-alpha-only", "tenant": "alpha",
	}, func(*event.Event) {})
	if err != nil {
		t.Fatalf("tenant run: %v", err)
	}
	if ans, _ := res["answer"].(string); ans != "tenant-alpha" {
		t.Errorf("answer = %q, want tenant-alpha", ans)
	}

	// The run is in alpha's journal, never the primary's.
	alphaJournal := readDir(t, filepath.Join(regRoot, "alpha", "journal"))
	primaryJournal := readDir(t, filepath.Join(primaryDir, "journal"))
	if !strings.Contains(alphaJournal, "routed-to-alpha-only") {
		t.Error("alpha journal should contain the routed run")
	}
	if strings.Contains(primaryJournal, "routed-to-alpha-only") {
		t.Error("primary journal must NOT contain a tenant-routed run")
	}

	// Routing to a tenant when the registry is present but the id is invalid errors.
	if _, err := c.Call(ctx, controlplane.CmdRun, map[string]any{"intent": "x", "tenant": "../evil"}); err == nil {
		t.Error("invalid tenant id should error")
	}
}

func readDir(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, _ := os.ReadFile(path)
		b.Write(data)
		return nil
	})
	return b.String()
}

func TestTenantDisabledWithoutRegistry(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// No SetTenants → every tenant command errors clearly.
	for _, cmd := range []string{
		controlplane.CmdTenantCreate, controlplane.CmdTenantList,
		controlplane.CmdTenantRelease, controlplane.CmdTenantRemove,
		controlplane.CmdTenantToken,
	} {
		if _, err := c.Call(ctx, cmd, map[string]any{"id": "alpha"}); err == nil {
			t.Errorf("%s should error when multi-tenancy is disabled", cmd)
		}
	}
}
