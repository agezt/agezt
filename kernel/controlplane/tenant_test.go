// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
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

func TestTenantDisabledWithoutRegistry(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// No SetTenants → every tenant command errors clearly.
	for _, cmd := range []string{
		controlplane.CmdTenantCreate, controlplane.CmdTenantList,
		controlplane.CmdTenantRelease, controlplane.CmdTenantRemove,
	} {
		if _, err := c.Call(ctx, cmd, map[string]any{"id": "alpha"}); err == nil {
			t.Errorf("%s should error when multi-tenancy is disabled", cmd)
		}
	}
}
