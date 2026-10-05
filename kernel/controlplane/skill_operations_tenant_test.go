// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
	"time"
)

func TestSkillOperationsSocketRetainPrimaryOnlyStore(t *testing.T) {
	primary, server, owner, dir := startPair(t, mock.New(mock.FinalText("unused")))
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	handle, err := registry.Acquire("acme", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tenant := handle.Kernel.(*runtime.Kernel)
	for label, k := range map[string]*runtime.Kernel{"primary-private": primary, "tenant-private": tenant} {
		if _, _, err := k.Forge().Create("seed", skill.CreateSpec{Name: label, Body: "body"}); err != nil {
			t.Fatal(err)
		}
	}
	client := tenantClient(t, dir, token)
	for _, cmd := range []string{controlplane.CmdSkillList, controlplane.CmdSkillGet, controlplane.CmdSkillHistory, controlplane.CmdSkillFiles, controlplane.CmdSkillReadFile, controlplane.CmdSkillHygiene, controlplane.CmdSkillPromote, controlplane.CmdSkillQuarantine, controlplane.CmdSkillArchive, controlplane.CmdSkillRevert, controlplane.CmdSkillRestore, controlplane.CmdSkillShare, controlplane.CmdSkillReassign, controlplane.CmdSkillImport} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"tenant": "acme", "id": "fixture", "path": "ref.md", "status": "draft", "name": "blocked", "body": "body"}); err == nil {
			t.Fatalf("tenant admitted %s", cmd)
		}
	}
	result, err := owner.Call(context.Background(), controlplane.CmdSkillList, map[string]any{"tenant": "acme"})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wire), "primary-private") || strings.Contains(string(wire), "tenant-private") {
		t.Fatalf("primary list selected tenant store: %s", wire)
	}
	if _, err := owner.Call(context.Background(), controlplane.CmdSkillImport, map[string]any{"tenant": "acme", "name": "primary-only-create", "body": "body"}); err != nil {
		t.Fatal(err)
	}
	p, err := primary.Forge().List()
	if err != nil {
		t.Fatal(err)
	}
	a, err := tenant.Forge().List()
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 2 || len(a) != 1 || a[0].Name != "tenant-private" {
		t.Fatalf("selected mutation store primary=%v tenant=%v", p, a)
	}
}
