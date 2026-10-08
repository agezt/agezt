// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/app"
	approster "github.com/agezt/agezt/kernel/app/roster"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/roster"
	"net"
	"testing"
	"time"
)

func TestAgentListInvalidationRejectsEmptyRosterCache(t *testing.T) {
	s := &Server{}
	profiles := []roster.Profile{{Slug: "owned", Enabled: true, UpdatedMS: 17}}
	s.rosterListOnce.Do(func() {
		s.rosterList = approster.NewList(func() []roster.Profile { return profiles }, func([]roster.Profile) map[string]map[string]any { return nil }, func() time.Time { return time.Unix(1, 0) })
	})
	read := func() Response {
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(time.Second))
		done := make(chan struct{})
		go func() {
			defer close(done)
			s.operationOnce.Do(func() {
				s.operations, s.operationErr = app.NewDispatcher(rosterListOperations, app.Dependencies{Auth: oauthSnapshotAuth{}, Router: oauthSnapshotRoute{s}})
			})
			handleAppOperation(&DispatchCtx{S: s, Conn: b, Req: Request{ID: "owned", Cmd: CmdAgentList}, Ctx: context.Background()})
		}()
		line, err := bufio.NewReader(a).ReadBytes(10)
		a.Close()
		b.Close()
		<-done
		if err != nil {
			t.Fatal(err)
		}
		var reply Response
		json.Unmarshal(line, &reply)
		return reply
	}
	first := read()
	if first.Type != RespResult || first.Result["total"] != float64(1) {
		t.Fatal(first)
	}
	profiles = nil
	s.invalidateAgentListCache()
	second := read()
	if second.Type != RespResult || second.Result["total"] != float64(0) || second.Result["enabled_count"] != float64(0) {
		t.Fatalf("EXPECTED: invalidated cache recomputes empty roster; ACTUAL: %+v", second)
	}
}

func TestAgentListNativeTypedRegistry(t *testing.T) {
	found := 0
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if spec.Name != CmdAgentList {
			continue
		}
		found++
		if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || !spec.ReadOnly || !spec.AllowUnknownInput || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.HTTP.Method != "GET" || spec.HTTP.Path != "/api/agents" {
			t.Fatalf("agent_list metadata=%+v", spec)
		}
	}
	wire, exists := commandRegistry[CmdAgentList]
	if found != 1 || !exists || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
		t.Fatalf("agent_list native wire found=%d metadata=%+v", found, wire)
	}
}
