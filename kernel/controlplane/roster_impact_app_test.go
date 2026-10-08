// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestAgentImpactTombstoneNativeTypedRegistry(t *testing.T) {
	for cmd, http := range map[string]opapi.HTTP{CmdAgentImpact: {Method: "GET", Path: "/api/agents/impact"}, CmdAgentTombstone: {}} {
		found := 0
		for _, operation := range registeredAppOperations() {
			spec := operation.Spec()
			if spec.Name != cmd {
				continue
			}
			found++
			if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || !spec.ReadOnly || !spec.AllowUnknownInput || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.HTTP != http {
				t.Fatalf("%s metadata=%+v", cmd, spec)
			}
		}
		wire, exists := commandRegistry[cmd]
		if found != 1 || !exists || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire found=%d metadata=%+v", cmd, found, wire)
		}
	}
}

// subagentImpact (the removal's retained sub-agent workflow refs) lists each
// child through the given lister and prefixes its labels.
func TestSubagentImpactListsEachChild(t *testing.T) {
	got := (&Server{}).subagentImpact([]roster.Profile{{Slug: "b", Name: "Bee"}, {Slug: "a"}}, func(_ *Server, p roster.Profile) []string {
		return []string{"w-" + p.Slug}
	})
	if strings.Join(got, ",") != "a: w-a,b: w-b" {
		t.Fatal(got)
	}
}

// The native impact source reads the live roster tree and subsystems; the wire
// keeps nil lists as null and the tombstone counts the same footprint.
func TestAgentImpactTombstoneNativeWire(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	for _, p := range []roster.Profile{{Slug: "lead", Soul: "s"}, {Slug: "child", Soul: "s", ParentAgent: "lead"}} {
		if _, err := k.AddProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	job, err := k.Schedules().Add("check disks", time.Hour, "", "test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.Schedules().SetAgent(job.ID, "lead"); err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, dir)
	s.token = "primary"
	call := func(cmd string) string {
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(3 * time.Second))
		done := make(chan struct{})
		go func() { defer close(done); s.handleConn(context.Background(), b) }()
		raw, _ := json.Marshal(Request{ID: "impact", Cmd: cmd, Token: "primary", Args: map[string]any{"ref": "lead"}})
		if _, err := a.Write(append(raw, 10)); err != nil {
			t.Fatal(err)
		}
		line, err := bufio.NewReader(a).ReadBytes(10)
		a.Close()
		<-done
		if err != nil {
			t.Fatal(err)
		}
		return string(line)
	}
	impact := call(CmdAgentImpact)
	for _, want := range []string{`"subagents":["child [parent]"]`, `"subagent_count":1`, `"schedules":["check disks (` + job.ID + `)"]`, `"schedule_count":1`, `"subagent_schedules":null`, `"subagent_schedule_count":0`, `"skills":null`, `"slug":"lead"`} {
		if !strings.Contains(impact, want) {
			t.Fatalf("impact missing %s: %s", want, impact)
		}
	}
	tombstone := call(CmdAgentTombstone)
	for _, want := range []string{`"result":{"tombstone":{`, `"schedules":1`, `"subagents":1`, `"retained_by_design":{"mailbox_messages":0,"workflow_refs":0}`} {
		if !strings.Contains(tombstone, want) {
			t.Fatalf("tombstone missing %s: %s", want, tombstone)
		}
	}
}
