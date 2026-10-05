// SPDX-License-Identifier: MIT

package controlplane

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

func TestTasteNativeExitCompleteTypedRegistry(t *testing.T) {
	type expected struct{ readOnly, tenant bool }
	want := map[string]expected{CmdTasteCreate: {}, CmdTasteDelete: {}, CmdTasteList: {readOnly: true}}
	seen := map[string]bool{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if !strings.HasPrefix(spec.Name, "taste_") {
			continue
		}
		entry, exists := want[spec.Name]
		if !exists || seen[spec.Name] {
			t.Fatalf("unexpected/duplicate taste operation %q", spec.Name)
		}
		seen[spec.Name] = true
		if spec.Input == nil || spec.Output == nil || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || spec.ReadOnly != entry.readOnly || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput {
			t.Fatalf("incomplete taste metadata=%+v", spec)
		}
		wire, exists := commandRegistry[spec.Name]
		if !exists || !wire.AppOwned || wire.ReadOnly != entry.readOnly || wire.TenantAllowed != entry.tenant || wire.TenantRouted != entry.tenant || wire.Streaming != StreamNone {
			t.Fatalf("taste native metadata=%+v", wire)
		}
		if entry.tenant {
			if spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant {
				t.Fatalf("taste log scope=%+v", spec)
			}
		} else if spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
			t.Fatalf("primary taste scope=%+v", spec)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("taste native coverage=%d/%d", len(seen), len(want))
	}
}
