// SPDX-License-Identifier: MIT

package controlplane

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestMemoryCommandMetadataComesFromCompleteAppRegistry(t *testing.T) {
	if len(memoryOperations) != 16 {
		t.Fatalf("memory operation count=%d", len(memoryOperations))
	}
	readOnly := map[string]bool{CmdMemoryGet: true, CmdMemoryList: true, CmdMemorySearch: true, CmdMemoryFindRelated: true, CmdMemoryAudit: true, CmdMemoryLog: true}
	tenant := map[string]bool{CmdMemoryAudit: true, CmdMemoryClean: true, CmdMemoryLog: true}
	for _, operation := range memoryOperations {
		spec := operation.Spec()
		wire := commandRegistry[spec.Name]
		if !wire.AppOwned || wire.Handler == nil || wire.ReadOnly != readOnly[spec.Name] || wire.TenantAllowed != tenant[spec.Name] || wire.TenantRouted != tenant[spec.Name] || wire.Streaming != StreamNone {
			t.Fatalf("memory adapter metadata=%+v", wire)
		}
		if tenant[spec.Name] && (spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant) {
			t.Fatalf("tenant spec=%+v", spec)
		}
	}
}

func TestMemoryAppSocketRequiresAuditBeforeMutations(t *testing.T) {
	for _, command := range []string{CmdMemoryAdd, CmdMemorySupersede, CmdMemoryForget, CmdMemoryPromote, CmdMemoryBulkForget, CmdMemoryPrune, CmdMemoryTidy, CmdMemoryClean, CmdMemoryConsolidate, CmdProfileRebuild} {
		t.Run(command, func(t *testing.T) {
			dir := t.TempDir()
			k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { k.Close() })
			rec, _, err := k.Memory().Remember("", memory.RememberSpec{Content: "owned fixture", Tags: map[string]string{"scope": "agent"}, Force: true})
			if err != nil {
				t.Fatal(err)
			}
			server := NewServer(k, dir)
			server.token = "primary"
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
			args := map[string]any{"id": rec.ID, "old_id": rec.ID, "content": "replacement fixture", "ids": []string{rec.ID}, "dry_run": false}
			response := callAppHost(t, server, Request{ID: command, Cmd: command, Token: "primary", Args: args})[0]
			if response.Type != RespError || !strings.Contains(response.Error, "journal") {
				t.Errorf("unavailable audit result=%+v", response)
			}
			after, found, err := k.Memory().Get(rec.ID)
			if err != nil || !found || k.Memory().Count() != 1 || after.Content != rec.Content || after.Tombstoned || after.SupersededBy != "" || after.Tags["scope"] != "agent" {
				t.Fatalf("mutation preceded audit: %+v found=%v count=%d err=%v", after, found, k.Memory().Count(), err)
			}
		})
	}
}
