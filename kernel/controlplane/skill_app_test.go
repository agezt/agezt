// SPDX-License-Identifier: MIT

package controlplane

import (
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
)

func TestSkillAppSocketRequiresAuditBeforeMutations(t *testing.T) {
	for _, cmd := range []string{CmdSkillPromote, CmdSkillQuarantine, CmdSkillArchive, CmdSkillRevert, CmdSkillRestore, CmdSkillShare, CmdSkillReassign, CmdSkillImport} {
		t.Run(cmd, func(t *testing.T) {
			dir := t.TempDir()
			k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
			if err != nil {
				t.Fatal(err)
			}
			defer k.Close()
			if _, err := k.AddProfile(roster.Profile{Slug: "reviewer"}); err != nil {
				t.Fatal(err)
			}
			sk, _, err := k.Forge().Create("seed", skill.CreateSpec{Name: "fixture", Body: "body", Agent: "writer", Resources: map[string][]byte{"ref.md": []byte("owned")}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := k.Forge().Promote("seed", sk.ID); err != nil {
				t.Fatal(err)
			}
			before, err := k.Forge().List()
			if err != nil {
				t.Fatal(err)
			}
			server := NewServer(k, dir)
			server.token = "primary"
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
			response := callAppHost(t, server, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"id": sk.ID, "reason": "owned", "status": "draft", "agent": "reviewer", "name": "new fixture", "body": "new body"}})[0]
			if cmd == CmdSkillPromote {
				for _, read := range []string{CmdSkillList, CmdSkillGet, CmdSkillHistory, CmdSkillFiles, CmdSkillReadFile, CmdSkillHygiene} {
					r := callAppHost(t, server, Request{ID: read, Cmd: read, Token: "primary", Args: map[string]any{"id": sk.ID, "path": "ref.md"}})[0]
					if r.Type != RespResult {
						t.Fatalf("read %s unexpectedly audited: %+v", read, r)
					}
				}
			}
			after, err := k.Forge().List()
			if err != nil {
				t.Fatal(err)
			}
			if response.Type != RespError || !strings.Contains(response.Error, "journal") || !reflect.DeepEqual(before, after) {
				t.Fatalf("EXPECTED: unavailable audit => error and unchanged skill store; ACTUAL: command=%s response=%+v changed=%v", cmd, response, !reflect.DeepEqual(before, after))
			}
		})
	}
}

func TestSkillCommandMetadataComesFromCompleteAppRegistry(t *testing.T) {
	if len(skillOperations) != 14 {
		t.Fatalf("skill operations=%d", len(skillOperations))
	}
	reads := map[string]bool{CmdSkillList: true, CmdSkillGet: true, CmdSkillHistory: true, CmdSkillFiles: true, CmdSkillReadFile: true, CmdSkillHygiene: true}
	for _, op := range skillOperations {
		spec := op.Spec()
		wire, found := commandRegistry[spec.Name]
		if !found || !wire.AppOwned || wire.ReadOnly != reads[spec.Name] || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("skill native metadata=%+v", wire)
		}
	}
}
