// SPDX-License-Identifier: MIT

package controlplane

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

func TestLifecycleNativeTypedRegistry(t *testing.T) {
	for cmd, readOnly := range map[string]bool{CmdHalt: false, CmdResume: false, CmdJournalVerify: true, CmdShutdown: false} {
		found := 0
		for _, operation := range registeredAppOperations() {
			spec := operation.Spec()
			if spec.Name != cmd {
				continue
			}
			found++
			if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || spec.ReadOnly != readOnly || !spec.AllowUnknownInput || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
				t.Fatalf("%s metadata=%+v", cmd, spec)
			}
		}
		wire, exists := commandRegistry[cmd]
		if found != 1 || !exists || !wire.AppOwned || wire.ReadOnly != readOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire found=%d metadata=%+v", cmd, found, wire)
		}
	}
}
