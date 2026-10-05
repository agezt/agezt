// SPDX-License-Identifier: MIT

package controlplane

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

func TestBoardNativeExitCompleteTypedRegistry(t *testing.T) {
	type expected struct{ readOnly bool }
	want := map[string]expected{CmdBoardRead: {readOnly: true}, CmdBoardHelp: {readOnly: true}, CmdBoardInbox: {readOnly: true}, CmdBoardGet: {readOnly: true}, CmdBoardReplies: {readOnly: true}, CmdBoardSend: {}, CmdBoardAck: {}}
	seen := map[string]bool{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if !strings.HasPrefix(spec.Name, "board_") {
			continue
		}
		entry, exists := want[spec.Name]
		if !exists || seen[spec.Name] {
			t.Fatalf("unexpected/duplicate board operation %q", spec.Name)
		}
		seen[spec.Name] = true
		if spec.Input == nil || spec.Output == nil || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || spec.ReadOnly != entry.readOnly || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput {
			t.Fatalf("incomplete board metadata=%+v", spec)
		}
		wire, exists := commandRegistry[spec.Name]
		if !exists || !wire.AppOwned || wire.ReadOnly != entry.readOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("board native metadata=%+v", wire)
		}
		if spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
			t.Fatalf("primary board scope=%+v", spec)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("board native coverage=%d/%d", len(seen), len(want))
	}
}
