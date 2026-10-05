// SPDX-License-Identifier: MIT

package controlplane

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/worldmodel"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestWorldCommandMetadataComesFromCompleteAppRegistry(t *testing.T) {
	if len(worldOperations) != 9 {
		t.Fatalf("world operation count=%d", len(worldOperations))
	}
	readOnly := map[string]bool{CmdWorldGet: true, CmdWorldList: true, CmdWorldResolve: true, CmdWorldNeighbors: true, CmdWorldLog: true}
	for _, operation := range worldOperations {
		spec := operation.Spec()
		wire := commandRegistry[spec.Name]
		tenant := spec.Name == CmdWorldLog
		if !wire.AppOwned || wire.ReadOnly != readOnly[spec.Name] || wire.TenantAllowed != tenant || wire.TenantRouted != tenant || wire.Streaming != StreamNone {
			t.Fatalf("world native metadata=%+v", wire)
		}
		if tenant && (spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant) {
			t.Fatalf("world log scope=%+v", spec)
		}
	}
}

func TestWorldAppSocketRequiresAuditBeforeMutations(t *testing.T) {
	for _, command := range []string{CmdWorldAdd, CmdWorldEdit, CmdWorldRelate, CmdWorldForget} {
		t.Run(command, func(t *testing.T) {
			dir := t.TempDir()
			k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { k.Close() })
			entity, _, err := k.World().Upsert("", worldmodel.UpsertSpec{Name: "owned fixture", Aliases: []string{"retained"}, Attrs: map[string]string{"old": "value"}})
			if err != nil {
				t.Fatal(err)
			}
			server := NewServer(k, dir)
			server.token = "primary"
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
			response := callAppHost(t, server, Request{ID: command, Cmd: command, Token: "primary", Args: map[string]any{"name": "new fixture", "id": entity.ID, "from": "owned fixture", "to": "new endpoint", "aliases": []string{"replacement"}, "attrs": map[string]string{"new": "value"}}})[0]
			if response.Type != RespError || !strings.Contains(response.Error, "journal") {
				t.Errorf("unavailable audit result=%+v", response)
			}
			after, found, err := k.World().Get(entity.ID)
			relations, rerr := k.World().Relations()
			if err != nil || rerr != nil || !found || k.World().Count() != 1 || after.Tombstoned || len(after.Aliases) != 1 || after.Aliases[0] != "retained" || after.Attrs["old"] != "value" || len(relations) != 0 {
				t.Fatalf("graph mutation preceded audit: %+v found=%v count=%d relations=%v err=%v/%v", after, found, k.World().Count(), relations, err, rerr)
			}
		})
	}
}
