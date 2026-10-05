// SPDX-License-Identifier: MIT

package controlplane

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

func TestProviderObservationMetadataComesFromTenantAppSpecs(t *testing.T) {
	if len(observationOperations) != 3 {
		t.Fatalf("observation operations = %d", len(observationOperations))
	}
	for _, operation := range observationOperations {
		spec := operation.Spec()
		wire := commandRegistry[spec.Name]
		if !wire.AppOwned || !wire.ReadOnly || !wire.TenantAllowed || !wire.TenantRouted || wire.Streaming != StreamNone || spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant || !spec.AllowUnknownInput {
			t.Fatalf("metadata = %+v %+v", spec, wire)
		}
	}
	probe := commandRegistry[CmdProviderProbe]
	if !probe.ReadOnly || probe.TenantAllowed || probe.TenantRouted {
		t.Fatalf("probe lost primary-only boundary: %+v", probe)
	}
}
