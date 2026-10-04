// SPDX-License-Identifier: MIT

package controlplane

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

func TestSystemCommandMetadataComesFromAppSpecs(t *testing.T) {
	if len(systemOperations) != 2 {
		t.Fatalf("system operations=%d", len(systemOperations))
	}
	for _, operation := range systemOperations {
		spec := operation.Spec()
		wire, ok := commandRegistry[spec.Name]
		if !ok || wire.ReadOnly != spec.ReadOnly || wire.TenantAllowed != (spec.Authz == opapi.OwnTenant) || wire.TenantRouted != (spec.Tenancy == opapi.CallerTenant) || wire.Streaming != StreamMode(spec.Stream) {
			t.Fatalf("metadata diverged: %+v %+v", spec, wire)
		}
		if !wire.AppOwned || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Input == nil || spec.Output == nil {
			t.Fatalf("pilot contract=%+v", spec)
		}
		if spec.HTTP.Method != "GET" || spec.HTTP.Path != "/api/"+spec.Name {
			t.Fatalf("HTTP hint=%+v", spec.HTTP)
		}
	}
}
