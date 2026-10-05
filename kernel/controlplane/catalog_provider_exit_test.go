// SPDX-License-Identifier: MIT

package controlplane

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

func TestCatalogProviderNativeExitCompleteTypedRegistry(t *testing.T) {
	type expected struct{ readOnly, tenant bool }
	want := map[string]expected{
		"catalog_sync": {}, "catalog_discover": {}, "catalog_list": {readOnly: true},
		"provider_connect": {}, "provider_reload": {},
		"provider_key_add": {}, "provider_key_activate": {}, "provider_key_remove": {}, "provider_key_list": {readOnly: true},
		"provider_oauth_start": {}, "provider_oauth_import": {}, "provider_oauth_logout": {}, "provider_oauth_status": {readOnly: true},
		"provider_log": {readOnly: true, tenant: true}, "provider_stats": {readOnly: true, tenant: true}, "provider_rejections": {readOnly: true, tenant: true},
		"provider_probe": {readOnly: true},
	}
	seen := make(map[string]bool)
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if !strings.HasPrefix(spec.Name, "catalog_") && !strings.HasPrefix(spec.Name, "provider_") {
			continue
		}
		entry, exists := want[spec.Name]
		if !exists || seen[spec.Name] {
			t.Fatalf("unexpected/duplicate operation %q", spec.Name)
		}
		seen[spec.Name] = true
		if spec.Input == nil || spec.Output == nil || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || spec.ReadOnly != entry.readOnly || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput {
			t.Fatalf("incomplete operation metadata %s: %+v", spec.Name, spec)
		}
		wire, exists := commandRegistry[spec.Name]
		if !exists || !wire.AppOwned || wire.ReadOnly != entry.readOnly || wire.TenantAllowed != entry.tenant || wire.TenantRouted != entry.tenant || wire.Streaming != StreamNone {
			t.Fatalf("native adapter metadata %s: %+v", spec.Name, wire)
		}
		if entry.tenant {
			if spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant {
				t.Fatalf("observation routing lost: %+v", spec)
			}
		} else if spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
			t.Fatalf("primary operation scope lost: %+v", spec)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("catalog/provider operation coverage = %d/%d", len(seen), len(want))
	}
}
