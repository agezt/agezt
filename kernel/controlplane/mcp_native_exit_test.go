// SPDX-License-Identifier: MIT
package controlplane

import (
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"reflect"
	"testing"
)

func TestMCPNativeSixOperationsTypedPolicyAndTypes(t *testing.T) {
	type signature struct {
		input, output reflect.Type
		read          bool
	}
	expected := map[string]signature{
		opapi.MCPList:       {reflect.TypeFor[apptools.MCPListInput](), reflect.TypeFor[apptools.MCPListOutput](), true},
		opapi.MCPAdd:        {reflect.TypeFor[apptools.MCPAddRequest](), reflect.TypeFor[apptools.MCPServerOutput](), false},
		opapi.MCPAttach:     {reflect.TypeFor[apptools.MCPRefRequest](), reflect.TypeFor[apptools.MCPAttachOutput](), false},
		opapi.MCPDetach:     {reflect.TypeFor[apptools.MCPRefRequest](), reflect.TypeFor[apptools.MCPDetachOutput](), false},
		opapi.MCPSetEnabled: {reflect.TypeFor[apptools.MCPSetEnabledRequest](), reflect.TypeFor[apptools.MCPServerOutput](), false},
		opapi.MCPRemove:     {reflect.TypeFor[apptools.MCPRefRequest](), reflect.TypeFor[apptools.MCPRemoveOutput](), false},
	}
	seen := map[string]int{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		signature, ok := expected[spec.Name]
		if !ok {
			continue
		}
		wire, ok := commandRegistry[spec.Name]
		if !ok || !wire.AppOwned || wire.ReadOnly != signature.read || wire.Streaming != StreamNone || wire.TenantAllowed || wire.TenantRouted || spec.ReadOnly != signature.read || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput || spec.Input != signature.input || spec.Output != signature.output || spec.Emission != nil || len(spec.EmissionSchema) != 0 {
			t.Fatal(spec, wire)
		}
		seen[spec.Name]++
	}
	if len(seen) != 6 || len(mcpCatalogOperations) != 1 || len(mcpLifecycleOperations) != 5 {
		t.Fatal(seen)
	}
	for _, count := range seen {
		if count != 1 {
			t.Fatal(seen)
		}
	}
}
