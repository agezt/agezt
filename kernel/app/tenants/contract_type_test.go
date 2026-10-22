// SPDX-License-Identifier: MIT

package tenants

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/types"
	"github.com/agezt/agezt/kernel/tenant"
)

// TestRowIsTheContractType: the tenant row IS the contract type, not a copy of
// it. The registry's snapshot therefore reaches the operator without a
// field-by-field projection, and the two sides cannot drift apart.
func TestRowIsTheContractType(t *testing.T) {
	if reflect.TypeOf(Row{}) != reflect.TypeOf(types.TenantInfo{}) {
		t.Fatalf("the tenant row drifted from the contract type: %v vs %v", reflect.TypeOf(Row{}), reflect.TypeOf(types.TenantInfo{}))
	}
	if reflect.TypeOf(tenant.Info{}) != reflect.TypeOf(types.TenantInfo{}) {
		t.Fatalf("the registry's row drifted from the contract type: %v vs %v", reflect.TypeOf(tenant.Info{}), reflect.TypeOf(types.TenantInfo{}))
	}
	// The registry's row and the application's row are the same type, so a
	// slice of one is a slice of the other.
	var rows []Row = []types.TenantInfo{{ID: "acme", BaseDir: "/tenants/acme", Open: true}}
	if len(rows) != 1 || rows[0].ID != "acme" {
		t.Fatal(rows)
	}
}

// TestListPublishesTheRegistryRowsUnchanged: the listing carries the
// registry's own rows — id, base dir and the open flag — and it stays an
// empty array (never null) when the registry holds no tenant.
func TestListPublishesTheRegistryRowsUnchanged(t *testing.T) {
	registry := &fakeRegistry{infos: []tenant.Info{
		{ID: "acme", BaseDir: "/tenants/acme", Open: true},
		{ID: "other", BaseDir: "/tenants/other"},
	}}
	service := New(registry, nil, func() time.Time { return time.Unix(0, 0) })
	out, err := service.List(context.Background(), ListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Count != 2 {
		t.Fatalf("count = %d, want 2", out.Count)
	}
	want := []tenant.Info{
		{ID: "acme", BaseDir: "/tenants/acme", Open: true},
		{ID: "other", BaseDir: "/tenants/other"},
	}
	if !reflect.DeepEqual(out.Tenants, want) {
		t.Fatal("the listing is not the registry's rows", out.Tenants)
	}
	raw, err := json.Marshal(out.Tenants)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `[{"id":"acme","base_dir":"/tenants/acme","open":true},{"id":"other","base_dir":"/tenants/other","open":false}]` {
		t.Fatal(string(raw))
	}
	empty := New(&fakeRegistry{}, nil, func() time.Time { return time.Unix(0, 0) })
	emptyOut, err := empty.List(context.Background(), ListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(emptyOut.Tenants)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "[]" {
		t.Fatal("an empty registry is an empty array, never null", string(raw))
	}
}
