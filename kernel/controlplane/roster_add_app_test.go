// SPDX-License-Identifier: MIT

package controlplane

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

func TestAgentAddNativeTypedRegistry(t *testing.T) {
	found := 0
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if spec.Name != CmdAgentAdd {
			continue
		}
		found++
		if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || spec.ReadOnly || !spec.AllowUnknownInput || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.HTTP != (opapi.HTTP{Method: "POST", Path: "/api/agents/add"}) {
			t.Fatalf("agent_add metadata=%+v", spec)
		}
	}
	wire, exists := commandRegistry[CmdAgentAdd]
	if found != 1 || !exists || !wire.AppOwned || wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
		t.Fatalf("agent_add native wire found=%d metadata=%+v", found, wire)
	}
}
