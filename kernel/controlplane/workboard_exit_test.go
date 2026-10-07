// SPDX-License-Identifier: MIT
package controlplane

import (
	"github.com/agezt/agezt/kernel/contract/opapi"
	"strings"
	"testing"
)

func TestWorkboardNativeExitCompleteTypedRegistry(t *testing.T) {
	reads := map[string]bool{CmdWorkboardList: true, CmdWorkboardLanes: true, CmdWorkboardShow: true, CmdWorkboardWatch: true}
	want := map[string]bool{}
	for _, name := range []string{CmdWorkboardList, CmdWorkboardLanes, CmdWorkboardShow, CmdWorkboardCreate, CmdWorkboardClaim, CmdWorkboardHeartbeat, CmdWorkboardComment, CmdWorkboardBlock, CmdWorkboardFail, CmdWorkboardUnblock, CmdWorkboardComplete, CmdWorkboardProve, CmdWorkboardSeat, CmdWorkboardArchive, CmdWorkboardLink, CmdWorkboardPolicy, CmdWorkboardDepend, CmdWorkboardReclaim, CmdWorkboardSweep, CmdWorkboardWatch, CmdWorkboardDispatch} {
		want[name] = reads[name]
	}
	seen := map[string]bool{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if !strings.HasPrefix(spec.Name, "workboard_") {
			continue
		}
		readOnly, exists := want[spec.Name]
		wire, registered := commandRegistry[spec.Name]
		if !exists || seen[spec.Name] || !registered || spec.Input == nil || spec.Output == nil || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || spec.ReadOnly != readOnly || spec.Stream != opapi.StreamNone || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || !wire.AppOwned || wire.ReadOnly != readOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("incomplete native exit: spec=%+v wire=%+v", spec, wire)
		}
		seen[spec.Name] = true
	}
	if len(seen) != 21 {
		t.Fatalf("workboard registry coverage=%d/21", len(seen))
	}
}
