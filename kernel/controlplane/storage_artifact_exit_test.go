// SPDX-License-Identifier: MIT
package controlplane

import (
	"github.com/agezt/agezt/kernel/contract/opapi"
	"strings"
	"testing"
)

func TestStorageArtifactNativeExitCompleteTypedRegistry(t *testing.T) {
	want := map[string]bool{CmdStorageStats: true, CmdArtifactGet: true, CmdArtifactList: true, CmdArtifactDelete: false, CmdArtifactCollect: false}
	seen := map[string]bool{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if spec.Name != CmdStorageStats && !strings.HasPrefix(spec.Name, "artifact_") {
			continue
		}
		readOnly, exists := want[spec.Name]
		wire, registered := commandRegistry[spec.Name]
		if !exists || seen[spec.Name] || !registered || spec.Input == nil || spec.Output == nil || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || spec.ReadOnly != readOnly || spec.Stream != opapi.StreamNone || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || !wire.AppOwned || wire.ReadOnly != readOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatal(spec, wire)
		}
		seen[spec.Name] = true
	}
	if len(seen) != 5 {
		t.Fatalf("storage/artifact native coverage=%d/5", len(seen))
	}
}
