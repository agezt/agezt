// SPDX-License-Identifier: MIT
package controlplane

import (
	"github.com/agezt/agezt/kernel/contract/opapi"
	"strings"
	"testing"
)

func TestOKRNativeExitCompleteTypedRegistry(t *testing.T) {
	want := map[string]bool{CmdOKRList: true, CmdOKRShow: true, CmdOKRCreate: false, CmdOKRKeyResult: false, CmdOKRLink: false, CmdOKRUnlink: false, CmdOKRArchive: false}
	seen := map[string]bool{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if !strings.HasPrefix(spec.Name, "okr_") {
			continue
		}
		readOnly, exists := want[spec.Name]
		wire, registered := commandRegistry[spec.Name]
		if !exists || seen[spec.Name] || !registered || spec.Input == nil || spec.Output == nil || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || spec.ReadOnly != readOnly || spec.Stream != opapi.StreamNone || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || !wire.AppOwned || wire.ReadOnly != readOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatal(spec, wire)
		}
		seen[spec.Name] = true
	}
	if len(seen) != 7 {
		t.Fatalf("OKR coverage=%d/7", len(seen))
	}
}
