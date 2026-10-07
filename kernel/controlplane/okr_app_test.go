// SPDX-License-Identifier: MIT
package controlplane

import (
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/okr"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workboard"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
	"time"
)

func okrAppFixture(t *testing.T) (*runtime.Kernel, *Server, okr.Objective, string, string, *mock.Provider) {
	t.Helper()
	dir := t.TempDir()
	provider := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	objective, err := k.OKR().Create(okr.CreateSpec{Title: "owned", Tenant: "team"}, time.UnixMilli(100))
	if err != nil {
		t.Fatal(err)
	}
	objective, err = k.OKR().AddKeyResult(objective.ID, "criterion", 1, time.UnixMilli(100))
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := k.Workboard().Create(workboard.CreateSpec{Title: "linked"}, time.UnixMilli(100))
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, dir)
	s.token = "primary"
	return k, s, objective, objective.KeyResults[0].ID, task.ID, provider
}
func TestOKRAppSocketRequiresAuditBeforeEveryMutation(t *testing.T) {
	for _, cmd := range []string{CmdOKRCreate, CmdOKRKeyResult, CmdOKRLink, CmdOKRUnlink, CmdOKRArchive} {
		t.Run(cmd, func(t *testing.T) {
			k, s, objective, kr, task, provider := okrAppFixture(t)
			before := k.OKR().List(okr.Filter{IncludeArchived: true})
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
			response := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"id": objective.ID, "title": "new", "key_result": kr, "task": task, "target": float64(2)}})[0]
			after := k.OKR().List(okr.Filter{IncludeArchived: true})
			if response.Type != RespError || !strings.Contains(response.Error, "journal") || !reflect.DeepEqual(before, after) || provider.CallCount() != 0 {
				t.Fatalf("EXPECTED: audit failure before OKR/provider effects; ACTUAL: command=%s response=%+v changed=%v provider=%d", cmd, response, !reflect.DeepEqual(before, after), provider.CallCount())
			}
		})
	}
}
func TestOKRCommandMetadataComesFromCompleteAppRegistry(t *testing.T) {
	if len(okrOperations) != 7 {
		t.Fatal("OKR operation count", len(okrOperations))
	}
	reads := map[string]bool{CmdOKRList: true, CmdOKRShow: true}
	seen := map[string]bool{}
	for _, operation := range okrOperations {
		spec := operation.Spec()
		wire, exists := commandRegistry[spec.Name]
		if !exists || !wire.AppOwned || wire.ReadOnly != reads[spec.Name] || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || spec.ReadOnly != reads[spec.Name] || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || spec.Input == nil || spec.Output == nil || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || !spec.AllowUnknownInput || seen[spec.Name] {
			t.Fatal(spec, wire)
		}
		seen[spec.Name] = true
	}
}
func TestOKRAppReadsRetainUnaryShapeWithoutAudit(t *testing.T) {
	k, s, objective, _, _, _ := okrAppFixture(t)
	for _, cmd := range []string{CmdOKRList, CmdOKRShow} {
		responses := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"id": objective.ID, "tenant": "team", "unused": true}})
		if len(responses) != 1 || responses[0].Type != RespResult {
			t.Fatal(cmd, responses)
		}
	}
	count := 0
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindOpInvoked && strings.HasPrefix(e.Subject, "op.okr_") {
			count++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("read audit count", count)
	}
	if err := k.Journal().Close(); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{CmdOKRList, CmdOKRShow} {
		response := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"id": objective.ID}})[0]
		if response.Type != RespResult {
			t.Fatal(cmd, response)
		}
	}
}
func TestOKRAppCreateKeepsOneAuditAndDefaultOrExplicitDomainIdentity(t *testing.T) {
	for _, explicit := range []string{"", " inbound "} {
		t.Run(explicit, func(t *testing.T) {
			k, s, _, _, _, _ := okrAppFixture(t)
			args := map[string]any{"title": "new", "owner": " writer ", "tenant": " team "}
			if explicit != "" {
				args["correlation_id"] = explicit
			}
			response := callAppHost(t, s, Request{ID: "create", Cmd: CmdOKRCreate, Token: "primary", Args: args})[0]
			if response.Type != RespResult {
				t.Fatal(response)
			}
			record := response.Result["objective"].(map[string]any)
			if record["owner"] != "writer" || record["tenant"] != "team" {
				t.Fatal(record)
			}
			id := record["id"].(string)
			rows := []*event.Event{}
			if err := k.Journal().Range(func(e *event.Event) error {
				if e.Subject == "op.okr_create" || e.Subject == "okr."+id {
					copy := *e
					rows = append(rows, &copy)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if len(rows) != 3 || rows[0].Kind != event.KindOpInvoked || rows[1].Kind != event.KindOKRObjectiveCreated || rows[2].Kind != event.KindOpCompleted || rows[0].CorrelationID == "" || rows[0].CorrelationID != rows[2].CorrelationID {
				t.Fatal(rows)
			}
			want := rows[0].CorrelationID
			if explicit != "" {
				want = strings.TrimSpace(explicit)
			}
			if rows[1].CorrelationID != want {
				t.Fatal("domain identity", rows[1].CorrelationID, want)
			}
		})
	}
}
func TestOKRAppAdmissionRetainsLenientTargetFiltersAndStrictBoolean(t *testing.T) {
	_, s, objective, _, _, _ := okrAppFixture(t)
	for _, raw := range []any{float64(2.9), nil, true, "3"} {
		response := callAppHost(t, s, Request{ID: "kr", Cmd: CmdOKRKeyResult, Token: "primary", Args: map[string]any{"id": objective.ID, "title": " criterion ", "target": raw, "unused": true}})[0]
		if response.Type != RespResult {
			t.Fatal(raw, response)
		}
		record := response.Result["objective"].(map[string]any)
		results := record["key_results"].([]any)
		last := results[len(results)-1].(map[string]any)
		want := float64(0)
		if _, ok := raw.(float64); ok {
			want = 2
		}
		target, _ := last["target"].(float64)
		if last["title"] != "criterion" || target != want {
			t.Fatal(raw, last)
		}
	}
	response := callAppHost(t, s, Request{ID: "list", Cmd: CmdOKRList, Token: "primary", Args: map[string]any{"status": "unknown", "limit": float64(.5)}})[0]
	if response.Type != RespResult || response.Result["count"] != float64(0) {
		t.Fatal(response)
	}
	for _, raw := range []any{nil, "true", float64(1)} {
		response := callAppHost(t, s, Request{ID: "bool", Cmd: CmdOKRList, Token: "primary", Args: map[string]any{"include_archived": raw}})[0]
		if response.Type != RespError {
			t.Fatal("invalid bool accepted", raw, response)
		}
	}
	response = callAppHost(t, s, Request{ID: "missing", Cmd: CmdOKRArchive, Token: "primary", Args: map[string]any{"id": "missing"}})[0]
	if response.Type != RespError || response.Error != "unknown objective: missing" {
		t.Fatal(response)
	}
}
