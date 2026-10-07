// SPDX-License-Identifier: MIT

package controlplane

import (
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workboard"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
	"time"
)

func workboardAppFixture(t *testing.T) (*runtime.Kernel, *Server, workboard.Task, workboard.Task, *mock.Provider) {
	t.Helper()
	dir := t.TempDir()
	provider := mock.New(mock.FinalText("owned completion"))
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	if _, err := k.AddProfile(roster.Profile{Slug: "writer"}); err != nil {
		t.Fatal(err)
	}
	task, _, err := k.Workboard().Create(workboard.CreateSpec{Title: "owned", Status: workboard.StatusReady, Assignee: "writer"}, time.UnixMilli(100))
	if err != nil {
		t.Fatal(err)
	}
	parent, _, err := k.Workboard().Create(workboard.CreateSpec{Title: "parent", Status: workboard.StatusReady}, time.UnixMilli(100))
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, dir)
	s.token = "primary"
	return k, s, task, parent, provider
}
func workboardMutationArgs(task, parent workboard.Task) map[string]any {
	return map[string]any{"id": task.ID, "title": "new task", "agent": "writer", "run_id": "owned-run", "author": "writer", "body": "owned comment", "actor": "writer", "reason": "owned reason", "type": "artifact", "target": "owned-artifact", "max_attempts": float64(3), "depends_on": parent.ID, "stale_after_ms": float64(1), "seat": "reader", "answer": "owned answer", "intent": "owned intent", "limit": float64(1)}
}
func TestWorkboardCommandMetadataComesFromCompleteAppRegistry(t *testing.T) {
	if len(workboardOperations) != 21 {
		t.Fatal("workboard operations", len(workboardOperations))
	}
	reads := map[string]bool{CmdWorkboardList: true, CmdWorkboardLanes: true, CmdWorkboardShow: true, CmdWorkboardWatch: true}
	seen := map[string]bool{}
	for _, operation := range workboardOperations {
		spec := operation.Spec()
		wire, ok := commandRegistry[spec.Name]
		if !ok || !wire.AppOwned || wire.ReadOnly != reads[spec.Name] || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || spec.ReadOnly != reads[spec.Name] || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || spec.Input == nil || spec.Output == nil || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || !spec.AllowUnknownInput || seen[spec.Name] {
			t.Fatalf("spec=%+v wire=%+v", spec, wire)
		}
		seen[spec.Name] = true
	}
}
func TestWorkboardAppSocketRequiresAuditBeforeEveryMutation(t *testing.T) {
	for _, cmd := range []string{CmdWorkboardCreate, CmdWorkboardClaim, CmdWorkboardHeartbeat, CmdWorkboardComment, CmdWorkboardBlock, CmdWorkboardFail, CmdWorkboardUnblock, CmdWorkboardComplete, CmdWorkboardProve, CmdWorkboardSeat, CmdWorkboardArchive, CmdWorkboardLink, CmdWorkboardPolicy, CmdWorkboardDepend, CmdWorkboardReclaim, CmdWorkboardSweep, CmdWorkboardDispatch} {
		t.Run(cmd, func(t *testing.T) {
			k, s, task, parent, provider := workboardAppFixture(t)
			before := k.Workboard().List(workboard.Filter{IncludeArchived: true})
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
			response := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: workboardMutationArgs(task, parent)})[0]
			after := k.Workboard().List(workboard.Filter{IncludeArchived: true})
			if response.Type != RespError || !strings.Contains(response.Error, "journal") || !reflect.DeepEqual(before, after) || provider.CallCount() != 0 {
				t.Fatalf("EXPECTED: audit failure => no workboard/provider effect; ACTUAL: cmd=%s response=%+v changed=%v provider=%d", cmd, response, !reflect.DeepEqual(before, after), provider.CallCount())
			}
		})
	}
}
func TestWorkboardAppReadsRemainUnauditedUnarySnapshots(t *testing.T) {
	k, s, task, _, _ := workboardAppFixture(t)
	for _, cmd := range []string{CmdWorkboardList, CmdWorkboardLanes, CmdWorkboardShow, CmdWorkboardWatch} {
		responses := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"id": task.ID, "unused": true}})
		if len(responses) != 1 || responses[0].Type != RespResult {
			t.Fatal(cmd, responses)
		}
	}
	audits := 0
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindOpInvoked && strings.HasPrefix(e.Subject, "op.workboard_") {
			audits++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if audits != 0 {
		t.Fatal("read audit count", audits)
	}
	if err := k.Journal().Close(); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{CmdWorkboardList, CmdWorkboardLanes, CmdWorkboardShow, CmdWorkboardWatch} {
		response := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"id": task.ID}})[0]
		if response.Type != RespResult {
			t.Fatal(cmd, response)
		}
	}
}
func TestWorkboardAppAdmissionRetainsAbsentNullAndLenientFields(t *testing.T) {
	k, s, task, _, _ := workboardAppFixture(t)
	cases := []struct {
		args     map[string]any
		accepted bool
	}{
		{map[string]any{"title": "owned", "idempotency_key": "dedupe", "tags": []any{" one ", true, ""}, "criteria": "first,second", "priority": float64(2.5), "unused": true}, true},
		{map[string]any{"title": "null maximum", "max_attempts": nil}, true},
		{map[string]any{"title": "wrong-type maximum", "max_attempts": true}, true},
		{map[string]any{"title": "bad policy", "max_attempts": nil, "escalate_to": "reviewer"}, false},
		{map[string]any{"title": true}, false},
		{map[string]any{"title": "bad seat", "seat": "missing"}, false},
	}
	for i, item := range cases {
		response := callAppHost(t, s, Request{ID: "create", Cmd: CmdWorkboardCreate, Token: "primary", Args: item.args})[0]
		if (response.Type == RespResult) != item.accepted {
			t.Fatal(item.args, response)
		}
		if !item.accepted {
			continue
		}
		record := response.Result["task"].(map[string]any)
		if i == 0 {
			if record["priority"] != float64(2) || !reflect.DeepEqual(record["tags"], []any{"one"}) {
				t.Fatal(record)
			}
		} else if _, present := record["retry_policy"]; present {
			t.Fatal("normalized empty policy returned", record)
		}
	}
	if len(k.Workboard().List(workboard.Filter{})) != 5 {
		t.Fatal("invalid create changed store")
	}
	for _, args := range []map[string]any{{"id": task.ID}, {"id": task.ID, "max_attempts": nil}, {"id": task.ID, "max_attempts": true}} {
		response := callAppHost(t, s, Request{ID: "policy", Cmd: CmdWorkboardPolicy, Token: "primary", Args: args})[0]
		if response.Type != RespError {
			t.Fatal("missing/invalid policy maximum accepted", args, response)
		}
	}
	response := callAppHost(t, s, Request{ID: "clear", Cmd: CmdWorkboardPolicy, Token: "primary", Args: map[string]any{"id": task.ID, "clear": true, "max_attempts": nil}})[0]
	if response.Type != RespResult {
		t.Fatal("clear did not override null maximum", response)
	}
	for _, cmd := range []string{CmdWorkboardList, CmdWorkboardLanes} {
		response := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"include_archived": nil}})[0]
		if response.Type != RespError {
			t.Fatal("null boolean accepted", response)
		}
	}
}

func TestWorkboardAppCreateOwnsOneAuditSpanAndDomainCorrelation(t *testing.T) {
	for _, explicit := range []string{"", " inbound "} {
		t.Run(explicit, func(t *testing.T) {
			k, s, _, _, _ := workboardAppFixture(t)
			args := map[string]any{"title": "new task"}
			if explicit != "" {
				args["correlation_id"] = explicit
			}
			response := callAppHost(t, s, Request{ID: "create", Cmd: CmdWorkboardCreate, Token: "primary", Args: args})[0]
			if response.Type != RespResult {
				t.Fatal(response)
			}
			id := response.Result["task"].(map[string]any)["id"].(string)
			records := []*event.Event{}
			if err := k.Journal().Range(func(e *event.Event) error {
				if e.Subject == "op.workboard_create" || e.Subject == "workboard."+id {
					copy := *e
					records = append(records, &copy)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if len(records) != 3 || records[0].Kind != event.KindOpInvoked || records[1].Kind != event.KindWorkboardTaskCreated || records[2].Kind != event.KindOpCompleted || records[0].CorrelationID == "" || records[0].CorrelationID != records[2].CorrelationID {
				t.Fatalf("audit/domain records=%+v", records)
			}
			want := records[0].CorrelationID
			if explicit != "" {
				want = strings.TrimSpace(explicit)
			}
			if records[1].CorrelationID != want {
				t.Fatalf("domain correlation=%q want=%q", records[1].CorrelationID, want)
			}
		})
	}
}
func TestWorkboardAppDispatchRetainsFreshOwnedRunAndAsyncReview(t *testing.T) {
	k, s, task, _, provider := workboardAppFixture(t)
	response := callAppHost(t, s, Request{ID: "dispatch", Cmd: CmdWorkboardDispatch, Token: "primary", Args: map[string]any{"id": task.ID, "agent": "writer", "intent": "owned intent", "correlation_id": "client-chosen"}})[0]
	if response.Type != RespResult || response.Result["accepted"] != true {
		t.Fatal(response)
	}
	corr, _ := response.Result["correlation_id"].(string)
	record := response.Result["task"].(map[string]any)
	if corr == "" || corr == "client-chosen" || record["status"] != "running" || record["claim"].(map[string]any)["run_id"] != corr {
		t.Fatal(response)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, _ := k.Workboard().Get(task.ID)
		if current.Status == workboard.StatusReview {
			if provider.CallCount() != 1 {
				t.Fatal("provider calls", provider.CallCount())
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	current, _ := k.Workboard().Get(task.ID)
	t.Fatalf("owned async run did not settle: %+v", current)
}
