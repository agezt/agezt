// SPDX-License-Identifier: MIT

package controlplane

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

func TestSkillNativeExitCompleteTypedRegistry(t *testing.T) {
	type expected struct{ readOnly bool }
	want := map[string]expected{CmdSkillList: {readOnly: true}, CmdSkillGet: {readOnly: true}, CmdSkillHistory: {readOnly: true}, CmdSkillFiles: {readOnly: true}, CmdSkillReadFile: {readOnly: true}, CmdSkillHygiene: {readOnly: true}, CmdSkillPromote: {}, CmdSkillQuarantine: {}, CmdSkillArchive: {}, CmdSkillRevert: {}, CmdSkillRestore: {}, CmdSkillShare: {}, CmdSkillReassign: {}, CmdSkillImport: {}}
	seen := map[string]bool{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if !strings.HasPrefix(spec.Name, "skill_") {
			continue
		}
		entry, exists := want[spec.Name]
		if !exists || seen[spec.Name] {
			t.Fatalf("unexpected/duplicate skill operation %q", spec.Name)
		}
		seen[spec.Name] = true
		if spec.Input == nil || spec.Output == nil || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || spec.ReadOnly != entry.readOnly || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput {
			t.Fatalf("incomplete skill metadata=%+v", spec)
		}
		wire, exists := commandRegistry[spec.Name]
		if !exists || !wire.AppOwned || wire.ReadOnly != entry.readOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("skill native metadata=%+v", wire)
		}
		if spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
			t.Fatalf("primary skill scope=%+v", spec)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("skill native coverage=%d/%d", len(seen), len(want))
	}
}
