// SPDX-License-Identifier: MIT

package controlplane

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

func TestMemoryNativeExitCompleteTypedRegistry(t *testing.T) {
	type expected struct{ readOnly, tenant bool }
	want := map[string]expected{
		opapi.MemoryGet: {readOnly: true}, opapi.MemoryList: {readOnly: true}, opapi.MemorySearch: {readOnly: true}, opapi.MemoryFindRelated: {readOnly: true},
		opapi.MemoryAdd: {}, opapi.MemorySupersede: {}, opapi.MemoryForget: {}, opapi.MemoryPromote: {}, opapi.MemoryBulkForget: {},
		opapi.MemoryPrune: {}, opapi.MemoryTidy: {}, opapi.MemoryAudit: {readOnly: true, tenant: true}, opapi.MemoryClean: {tenant: true},
		opapi.MemoryLog: {readOnly: true, tenant: true}, opapi.MemoryConsolidate: {}, opapi.ProfileRebuild: {},
	}
	seen := map[string]bool{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if !strings.HasPrefix(spec.Name, "memory_") && spec.Name != opapi.ProfileRebuild {
			continue
		}
		entry, exists := want[spec.Name]
		if !exists || seen[spec.Name] {
			t.Fatalf("unexpected/duplicate memory operation %q", spec.Name)
		}
		seen[spec.Name] = true
		if spec.Input == nil || spec.Output == nil || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || spec.ReadOnly != entry.readOnly || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput {
			t.Fatalf("incomplete memory metadata=%+v", spec)
		}
		wire, exists := commandRegistry[spec.Name]
		if !exists || !wire.AppOwned || wire.ReadOnly != entry.readOnly || wire.TenantAllowed != entry.tenant || wire.TenantRouted != entry.tenant || wire.Streaming != StreamNone {
			t.Fatalf("native memory metadata=%+v", wire)
		}
		if entry.tenant {
			if spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant {
				t.Fatalf("routed memory scope=%+v", spec)
			}
		} else if spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
			t.Fatalf("primary memory scope=%+v", spec)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("memory native coverage=%d/%d", len(seen), len(want))
	}
}
