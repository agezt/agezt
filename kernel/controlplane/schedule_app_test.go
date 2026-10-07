// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
	"time"
)

func scheduleAppFixture(t *testing.T) (*runtime.Kernel, *Server, string, *mock.Provider) {
	t.Helper()
	dir := t.TempDir()
	provider := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	entry, err := k.Schedules().Add("owned", time.Hour, "", "operator", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, dir)
	s.token = "primary"
	return k, s, entry.ID, provider
}
func TestScheduleNativeMetadataComesFromTenTypedAppSpecs(t *testing.T) {
	reads := map[string]bool{CmdScheduleList: true, CmdScheduleSystemTasks: true, CmdScheduleTest: true, CmdScheduleFires: true, CmdScheduleStats: true}
	tenants := map[string]bool{CmdScheduleFires: true, CmdScheduleStats: true}
	want := map[string]bool{}
	for _, name := range []string{CmdScheduleAdd, CmdScheduleList, CmdScheduleSystemTasks, CmdScheduleRemove, CmdScheduleRun, CmdScheduleEnable, CmdScheduleEdit, CmdScheduleTest, CmdScheduleFires, CmdScheduleStats} {
		want[name] = reads[name]
	}
	seen := map[string]bool{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if !strings.HasPrefix(spec.Name, "schedule_") {
			continue
		}
		readOnly, known := want[spec.Name]
		wire, exists := commandRegistry[spec.Name]
		authz, tenancy := opapi.PrimaryOnly, opapi.Primary
		if tenants[spec.Name] {
			authz, tenancy = opapi.OwnTenant, opapi.CallerTenant
		}
		if !known || seen[spec.Name] || !exists || !wire.AppOwned || wire.ReadOnly != readOnly || wire.TenantAllowed != tenants[spec.Name] || wire.TenantRouted != tenants[spec.Name] || wire.Streaming != StreamNone || spec.Authz != authz || spec.Tenancy != tenancy || spec.ReadOnly != readOnly || spec.Stream != opapi.StreamNone || spec.Input == nil || spec.Output == nil || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || !spec.AllowUnknownInput {
			t.Fatal(spec, wire)
		}
		seen[spec.Name] = true
	}
	if len(seen) != 10 || len(scheduleOperations) != 10 {
		t.Fatal(seen, len(scheduleOperations))
	}
}
func TestScheduleAppRequiresActualAuditBeforeAllFiveMutations(t *testing.T) {
	for _, cmd := range []string{CmdScheduleAdd, CmdScheduleRemove, CmdScheduleRun, CmdScheduleEnable, CmdScheduleEdit} {
		t.Run(cmd, func(t *testing.T) {
			k, s, id, provider := scheduleAppFixture(t)
			before := k.Schedules().List()
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
			responses := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"id": id, "intent": "new owned", "interval_sec": float64(60), "enabled": false}})
			after := k.Schedules().List()
			if len(responses) != 1 || responses[0].Type != RespError || !strings.Contains(responses[0].Error, "journal") || !reflect.DeepEqual(before, after) || provider.CallCount() != 0 {
				t.Fatalf("EXPECTED: unavailable mandatory audit blocks all cadence/provider effects; ACTUAL: cmd=%s response=%+v changed=%v calls=%d", cmd, responses, !reflect.DeepEqual(before, after), provider.CallCount())
			}
		})
	}
}
func TestScheduleAppReadsRetainUnauditedUnaryShapesAndMissingEditAdmission(t *testing.T) {
	k, s, id, provider := scheduleAppFixture(t)
	for _, cmd := range []string{CmdScheduleList, CmdScheduleSystemTasks, CmdScheduleTest, CmdScheduleFires, CmdScheduleStats} {
		responses := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"id": id, "unused": true}})
		if len(responses) != 1 || responses[0].Type != RespResult {
			t.Fatal(cmd, responses)
		}
	}
	audits := 0
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindOpInvoked && strings.HasPrefix(e.Subject, "op.schedule_") {
			audits++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if audits != 0 || provider.CallCount() != 0 {
		t.Fatal(audits, provider.CallCount())
	}
	responses := callAppHost(t, s, Request{ID: "missing", Cmd: CmdScheduleEdit, Token: "primary", Args: map[string]any{"id": "missing", "target": true, "agent": true, "interval_sec": "bad"}})
	if len(responses) != 1 || responses[0].Type != RespResult || !reflect.DeepEqual(responses[0].Result, map[string]any{"updated": false}) {
		t.Fatal(responses)
	}
}
func TestScheduleEnableOperatorActionJoinsOwnedAuditCorrelationAndIgnoresRawSpoof(t *testing.T) {
	k, s, id, _ := scheduleAppFixture(t)
	responses := callAppHost(t, s, Request{ID: "enable", Cmd: CmdScheduleEnable, Token: "primary", Args: map[string]any{"id": id, "enabled": "TRUE", "corr": "client-spoof", "correlation_id": "client-spoof"}})
	if len(responses) != 1 || responses[0].Type != RespResult {
		t.Fatal(responses)
	}
	invoked, completed, action := []*event.Event{}, []*event.Event{}, []*event.Event{}
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Subject == "op.schedule_enable" && e.Kind == event.KindOpInvoked {
			invoked = append(invoked, e)
		}
		if e.Subject == "op.schedule_enable" && e.Kind == event.KindOpCompleted {
			completed = append(completed, e)
		}
		if e.Subject == "schedule.enable" {
			action = append(action, e)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(invoked) != 1 || len(completed) != 1 || len(action) != 1 {
		t.Fatal(invoked, completed, action)
	}
	corr := invoked[0].CorrelationID
	if corr == "" || corr == "client-spoof" || completed[0].CorrelationID != corr || action[0].CorrelationID != corr || action[0].Actor != "controlplane" {
		t.Fatal(invoked, completed, action)
	}
	var payload map[string]any
	if err := json.Unmarshal(action[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["id"] != id || payload["enabled"] != true || payload["action"] != "resumed" {
		t.Fatal(payload)
	}
}

func TestScheduleAppOutputSchemasRetainRawPayloadAndRequiredPresence(t *testing.T) {
	_, server, id, _ := scheduleAppFixture(t)
	responses := callAppHost(t, server, Request{ID: "add", Cmd: CmdScheduleAdd, Token: "primary", Args: map[string]any{"intent": "owned", "interval_sec": float64(60)}})
	if len(responses) != 1 || responses[0].Type != RespResult {
		t.Fatal(responses)
	}
	var declared json.RawMessage
	var edit json.RawMessage
	for _, operation := range scheduleOperations {
		spec := operation.Spec()
		if spec.Name == CmdScheduleAdd {
			declared = spec.OutputSchema
		}
		if spec.Name == CmdScheduleEdit {
			edit = spec.OutputSchema
		}
	}
	row := responses[0].Result
	for _, payload := range []any{nil, map[string]any{"owned": true}, []any{false, float64(0)}, "raw"} {
		row["payload"] = payload
		raw, _ := json.Marshal(row)
		if err := schema.ValidateJSON(declared, raw); err != nil {
			t.Fatal(payload, err)
		}
	}
	delete(row, "payload")
	raw, _ := json.Marshal(row)
	if schema.ValidateJSON(declared, raw) == nil {
		t.Fatal("required payload presence not enforced")
	}
	if err := schema.ValidateJSON(edit, json.RawMessage(`{"updated":false}`)); err != nil {
		t.Fatal(err)
	}
	if schema.ValidateJSON(edit, json.RawMessage(`{}`)) == nil {
		t.Fatal("edit updated presence not enforced")
	}
	responses = callAppHost(t, server, Request{ID: "edit", Cmd: CmdScheduleEdit, Token: "primary", Args: map[string]any{"id": id}})
	if len(responses) != 1 || responses[0].Type != RespResult {
		t.Fatal(responses)
	}
}
