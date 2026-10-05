// SPDX-License-Identifier: MIT

package controlplane

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

func TestWorldNativeExitCompleteTypedRegistry(t *testing.T) {
	type expected struct{ readOnly, tenant bool }
	want := map[string]expected{CmdWorldAdd: {}, CmdWorldEdit: {}, CmdWorldRelate: {}, CmdWorldForget: {}, CmdWorldGet: {readOnly: true}, CmdWorldList: {readOnly: true}, CmdWorldResolve: {readOnly: true}, CmdWorldNeighbors: {readOnly: true}, CmdWorldLog: {readOnly: true, tenant: true}}
	seen := map[string]bool{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if !strings.HasPrefix(spec.Name, "world_") {
			continue
		}
		entry, exists := want[spec.Name]
		if !exists || seen[spec.Name] {
			t.Fatalf("unexpected/duplicate world operation %q", spec.Name)
		}
		seen[spec.Name] = true
		if spec.Input == nil || spec.Output == nil || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || spec.ReadOnly != entry.readOnly || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput {
			t.Fatalf("incomplete world metadata=%+v", spec)
		}
		wire, exists := commandRegistry[spec.Name]
		if !exists || !wire.AppOwned || wire.ReadOnly != entry.readOnly || wire.TenantAllowed != entry.tenant || wire.TenantRouted != entry.tenant || wire.Streaming != StreamNone {
			t.Fatalf("world native metadata=%+v", wire)
		}
		if entry.tenant {
			if spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant {
				t.Fatalf("world log scope=%+v", spec)
			}
		} else if spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
			t.Fatalf("primary world scope=%+v", spec)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("world native coverage=%d/%d", len(seen), len(want))
	}
}
