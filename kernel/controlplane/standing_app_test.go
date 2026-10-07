// SPDX-License-Identifier: MIT
package controlplane

import (
	"encoding/json"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
)

func standingAppFixture(t *testing.T) (*runtime.Kernel, *Server, string, *mock.Provider) {
	t.Helper()
	dir := t.TempDir()
	provider := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	order, err := k.AddStanding(standing.Order{Name: "owned", Triggers: []standing.Trigger{{Type: standing.TriggerEvent, Subject: "fixture"}}})
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, dir)
	s.token = "primary"
	return k, s, order.ID, provider
}
func TestStandingNativeMetadataComesFromSevenTypedAppSpecs(t *testing.T) {
	want := map[string]bool{CmdStandingList: true, CmdStandingWhy: true, CmdStandingAdd: false, CmdStandingEdit: false, CmdStandingSetEnabled: false, CmdStandingRemove: false, CmdStandingFire: false}
	seen := map[string]bool{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if !strings.HasPrefix(spec.Name, "standing_") {
			continue
		}
		read, known := want[spec.Name]
		wire, exists := commandRegistry[spec.Name]
		if !known || seen[spec.Name] || !exists || !wire.AppOwned || wire.ReadOnly != read || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || spec.ReadOnly != read || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || spec.Input == nil || spec.Output == nil || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || !spec.AllowUnknownInput {
			t.Fatal(spec, wire)
		}
		seen[spec.Name] = true
	}
	if len(seen) != 7 || len(standingOperations) != 7 {
		t.Fatal(seen, len(standingOperations))
	}
}
func TestStandingAppRequiresActualAuditBeforeAllFiveMutations(t *testing.T) {
	for _, cmd := range []string{CmdStandingAdd, CmdStandingEdit, CmdStandingSetEnabled, CmdStandingRemove, CmdStandingFire} {
		t.Run(cmd, func(t *testing.T) {
			k, s, id, provider := standingAppFixture(t)
			fires := 0
			s.SetStandingFire(func(string) bool { fires++; return true })
			before := k.Standing().List()
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
			responses := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"id": id, "name": "edited", "enabled": false, "order": map[string]any{"name": "added", "triggers": []any{map[string]any{"type": "event", "subject": "fixture"}}}}})
			after := k.Standing().List()
			if len(responses) != 1 || responses[0].Type != RespError || !strings.Contains(responses[0].Error, "journal") || !reflect.DeepEqual(before, after) || fires != 0 || provider.CallCount() != 0 {
				t.Fatalf("EXPECTED: unavailable audit blocks standing state and callback; ACTUAL: %s %+v changed=%v fires=%d provider=%d", cmd, responses, !reflect.DeepEqual(before, after), fires, provider.CallCount())
			}
		})
	}
}
func TestStandingAppReadsUnauditedAndFireUsesLatestInjectedCallback(t *testing.T) {
	k, s, id, provider := standingAppFixture(t)
	for _, cmd := range []string{CmdStandingList, CmdStandingWhy} {
		responses := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"id": id, "unused": true}})
		if len(responses) != 1 || responses[0].Type != RespResult {
			t.Fatal(cmd, responses)
		}
	}
	audit := 0
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindOpInvoked && strings.HasPrefix(e.Subject, "op.standing_") {
			audit++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if audit != 0 {
		t.Fatal(audit)
	}
	first := callAppHost(t, s, Request{ID: "unavailable", Cmd: CmdStandingFire, Token: "primary", Args: map[string]any{"id": id}})
	if len(first) != 1 || first[0].Type != RespError || !strings.Contains(first[0].Error, "not available") {
		t.Fatal(first)
	}
	calls := []string{}
	s.SetStandingFire(func(ref string) bool { calls = append(calls, ref); return false })
	out := callAppHost(t, s, Request{ID: "declined", Cmd: CmdStandingFire, Token: "primary", Args: map[string]any{"id": id}})
	if len(out) != 1 || out[0].Type != RespResult || !reflect.DeepEqual(out[0].Result, map[string]any{"fired": false, "id": id}) || !reflect.DeepEqual(calls, []string{id}) || provider.CallCount() != 0 {
		t.Fatal(out, calls, provider.CallCount())
	}
	invoked, completed, failed := 0, 0, 0
	corrs := map[string]string{}
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "op.standing_fire" {
			return nil
		}
		if e.CorrelationID == "" {
			t.Fatal(e)
		}
		switch e.Kind {
		case event.KindOpInvoked:
			invoked++
			corrs[e.CorrelationID] = "begin"
		case event.KindOpCompleted:
			completed++
			if corrs[e.CorrelationID] != "begin" {
				t.Fatal(e)
			}
		case event.KindOpFailed:
			failed++
			if corrs[e.CorrelationID] != "begin" {
				t.Fatal(e)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if invoked != 2 || completed != 1 || failed != 1 {
		t.Fatal(invoked, completed, failed)
	}
}
func TestStandingAppPresenceValidationAndOutputSchemas(t *testing.T) {
	_, s, id, _ := standingAppFixture(t)
	for _, value := range []any{"true", "1", float64(1), nil, true, false} {
		out := callAppHost(t, s, Request{ID: "enable", Cmd: CmdStandingSetEnabled, Token: "primary", Args: map[string]any{"id": id, "enabled": value}})
		want, _ := value.(bool)
		if len(out) != 1 || out[0].Type != RespResult || out[0].Result["order"].(map[string]any)["enabled"] != want {
			t.Fatal(value, out)
		}
	}
	for _, args := range []map[string]any{{"id": "missing", "agent": "unknown", "assure": "wrong"}, {"id": "missing", "name": nil, "agent": "unknown"}} {
		out := callAppHost(t, s, Request{ID: "edit", Cmd: CmdStandingEdit, Token: "primary", Args: args})
		if len(out) != 1 || out[0].Type != RespError || (!strings.Contains(out[0].Error, "args.assure must be a number") && !strings.Contains(out[0].Error, "args.name must be a string")) {
			t.Fatal(args, out)
		}
	}
	for _, operation := range standingOperations {
		spec := operation.Spec()
		args := map[string]any{"id": "missing"}
		if spec.Name == CmdStandingList {
			args["id"] = id
		}
		if spec.Name == CmdStandingFire {
			s.SetStandingFire(func(string) bool { return true })
		}
		if spec.Name == CmdStandingAdd {
			args["order"] = map[string]any{"name": "added", "triggers": []any{map[string]any{"type": "event", "subject": "fixture"}}}
		}
		if spec.Name == CmdStandingSetEnabled {
			args["id"] = id
		}
		out := callAppHost(t, s, Request{ID: spec.Name, Cmd: spec.Name, Token: "primary", Args: args})
		if len(out) != 1 || out[0].Type != RespResult {
			t.Fatal(spec.Name, out)
		}
		raw, _ := json.Marshal(out[0].Result)
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatal(spec.Name, err)
		}
		if spec.Name == CmdStandingWhy && out[0].Result["events"] != nil {
			t.Fatal(out)
		}
		if spec.Name == CmdStandingEdit && !reflect.DeepEqual(out[0].Result, map[string]any{"updated": false}) {
			t.Fatal(out)
		}
		if spec.Name == CmdStandingFire {
			var node map[string]any
			_ = json.Unmarshal(spec.OutputSchema, &node)
			required := node["required"].([]any)
			if !reflect.DeepEqual(required, []any{"fired", "id"}) {
				t.Fatal(required)
			}
		}
	}
}
func TestStandingAppAuditKeepsRuntimeLifecycleIdentity(t *testing.T) {
	k, s, id, _ := standingAppFixture(t)
	out := callAppHost(t, s, Request{ID: "edit", Cmd: CmdStandingEdit, Token: "primary", Args: map[string]any{"id": id, "name": "edited", "correlation_id": "spoof"}})
	if len(out) != 1 || out[0].Type != RespResult {
		t.Fatal(out)
	}
	var invoked, completed, domain []*event.Event
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Subject == "op.standing_edit" {
			if e.Kind == event.KindOpInvoked {
				invoked = append(invoked, e)
			}
			if e.Kind == event.KindOpCompleted {
				completed = append(completed, e)
			}
		}
		if e.Kind == event.KindStandingUpdated {
			domain = append(domain, e)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(invoked) != 1 || len(completed) != 1 || len(domain) != 1 || invoked[0].CorrelationID == "" || invoked[0].CorrelationID == "spoof" || completed[0].CorrelationID != invoked[0].CorrelationID || domain[0].CorrelationID != "" {
		t.Fatal(invoked, completed, domain)
	}
}
