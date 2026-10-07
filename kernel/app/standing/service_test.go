// SPDX-License-Identifier: MIT

package standing_test

import (
	"context"
	"encoding/json"
	"errors"
	appstanding "github.com/agezt/agezt/kernel/app/standing"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	orders "github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
)

type standingPort struct {
	rows    []orders.Order
	current orders.Order
	found   bool
	calls   []string
	cause   error
	removed bool
	mutated orders.Order
}

func (p *standingPort) List() []orders.Order { return p.rows }
func (p *standingPort) Get(string) (orders.Order, bool) {
	p.calls = append(p.calls, "get")
	return p.current, p.found
}
func (p *standingPort) AddStanding(o orders.Order) (orders.Order, error) {
	p.calls = append(p.calls, "add")
	p.mutated = o
	return p.current, p.cause
}
func (p *standingPort) UpdateStanding(_ string, mutate func(*orders.Order)) (orders.Order, bool, error) {
	p.calls = append(p.calls, "update")
	p.mutated = p.current
	mutate(&p.mutated)
	return p.mutated, p.found, p.cause
}
func (p *standingPort) SetStandingEnabled(_ string, enabled bool) (orders.Order, error) {
	p.calls = append(p.calls, "enable")
	p.mutated = p.current
	p.mutated.Enabled = enabled
	return p.mutated, p.cause
}
func (p *standingPort) RemoveStanding(string) (bool, error) {
	p.calls = append(p.calls, "remove")
	return p.removed, p.cause
}
func TestStandingServiceListRetainsOrderCountsOmissionsAndTargetAnnotations(t *testing.T) {
	port := &standingPort{rows: []orders.Order{{ID: "plain", Enabled: true}, {ID: "ready", Enabled: false, Agent: " ready ", Triggers: []orders.Trigger{{Type: orders.TriggerEvent, Subject: "owned"}}, CooldownSec: 60}, {ID: "blocked", Enabled: true, Agent: "missing"}}}
	service := appstanding.New(port, port, appstanding.Host{Agent: func(ref string) (roster.Profile, bool) {
		return roster.Profile{Slug: ref, Enabled: true}, ref == "ready"
	}})
	out, err := service.List(context.Background(), appstanding.ListInput{})
	if err != nil || out.Count != 3 || out.EnabledCount != 2 || out.Orders[0].ID != "plain" || out.Orders[0].TargetStatus != "" || out.Orders[1].TargetStatus != "ready" || out.Orders[1].FrequencyWarning != "event cooldown is below the default 15m guard" || out.Orders[2].TargetStatus != "blocked" || out.Orders[2].TargetError != "unknown standing agent: missing" {
		t.Fatal(out, err)
	}
	raw, _ := json.Marshal(out.Orders[0])
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	for _, key := range []string{"id", "name", "enabled", "triggers", "initiative", "created_ms", "updated_ms"} {
		if _, ok := fields[key]; !ok {
			t.Fatal(key, string(raw))
		}
	}
	for _, key := range []string{"agent", "plan", "frequency_warning", "target_status", "target_error"} {
		if _, ok := fields[key]; ok {
			t.Fatal(key, string(raw))
		}
	}
	empty, err := appstanding.New(&standingPort{}, port, appstanding.Host{}).List(context.Background(), appstanding.ListInput{})
	raw, _ = json.Marshal(empty)
	if err != nil || string(raw) != `{"orders":[],"count":0,"enabled_count":0}` {
		t.Fatal(empty, err, string(raw))
	}
}
func TestStandingFrequencyWarningRetainsCronPriorityExactFormsAndEventThreshold(t *testing.T) {
	for _, tc := range []struct {
		schedule string
		cooldown int64
		event    bool
		want     string
	}{{"* * * * *", 60, true, "cron trigger may wake this standing order every minute"}, {" */1 * * * * ", 900, false, "cron trigger may wake this standing order every minute"}, {"0/1 * * * *", 0, false, "cron trigger may wake this standing order every minute"}, {"*/2 * * * *", 899, true, "event cooldown is below the default 15m guard"}, {"0 * * * *", 900, true, ""}, {"", 0, true, ""}, {"* * * * *", 60, false, "cron trigger may wake this standing order every minute"}, {"0 * * * *", 60, false, ""}} {
		o := orders.Order{CooldownSec: tc.cooldown, Triggers: []orders.Trigger{{Type: orders.TriggerCron, Schedule: tc.schedule}}}
		if tc.event {
			o.Triggers = append(o.Triggers, orders.Trigger{Type: orders.TriggerEvent})
		}
		if got := appstanding.FrequencyWarning(o); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}
func TestStandingServiceValidationPrecedesWritesButPauseSkipsAgentGate(t *testing.T) {
	no := false
	for _, mode := range []string{"unknown", "retired", "paused", "managed", "valid", "blank"} {
		for _, operation := range []string{"add", "edit", "resume", "pause"} {
			t.Run(mode+"/"+operation, func(t *testing.T) {
				port := &standingPort{found: true, current: orders.Order{ID: "owned", Agent: " alias "}}
				profile := roster.Profile{Slug: "canonical", Enabled: true}
				switch mode {
				case "retired":
					profile.Retired = true
					profile.Enabled = false
				case "paused":
					profile.Enabled = false
				case "managed":
					profile.DirectCallable = &no
				case "blank":
					port.current.Agent = " "
				}
				valid := mode == "valid" || mode == "blank"
				service := appstanding.New(port, port, appstanding.Host{Agent: func(ref string) (roster.Profile, bool) {
					port.calls = append(port.calls, "validate")
					if ref != "alias" {
						t.Fatal(ref)
					}
					return profile, mode != "unknown"
				}, ManagedDirectError: func(p roster.Profile, action string) string {
					if p.Slug != "canonical" || action != "called" {
						t.Fatal(p, action)
					}
					return "selected managed refusal"
				}})
				var err error
				switch operation {
				case "add":
					_, err = service.Add(context.Background(), appstanding.AddInput{Order: port.current})
				case "edit":
					_, err = service.Edit(context.Background(), appstanding.EditInput{ID: "owned", Agent: appstanding.TextField{Value: port.current.Agent, Present: true}})
				case "resume":
					_, err = service.SetEnabled(context.Background(), appstanding.EnableInput{ID: "owned", Enabled: true})
				case "pause":
					_, err = service.SetEnabled(context.Background(), appstanding.EnableInput{ID: "owned", Enabled: false})
				}
				if operation == "pause" {
					if err != nil || !reflect.DeepEqual(port.calls, []string{"enable"}) {
						t.Fatal(err, port.calls)
					}
					return
				}
				if !valid {
					if err == nil || len(port.calls) == 0 || port.calls[len(port.calls)-1] != "validate" {
						t.Fatal(err, port.calls)
					}
					switch mode {
					case "unknown":
						if err.Error() != "unknown standing agent: alias" {
							t.Fatal(err)
						}
					case "retired":
						if err.Error() != "standing agent canonical is retired" {
							t.Fatal(err)
						}
					case "paused":
						if err.Error() != "standing agent canonical is paused" {
							t.Fatal(err)
						}
					case "managed":
						if err.Error() != "standing selected managed refusal" {
							t.Fatal(err)
						}
					}
				} else if err != nil {
					t.Fatal(err)
				}
				if operation == "add" && valid && port.mutated.Agent != port.current.Agent {
					t.Fatal("add normalized legacy raw agent", port.mutated)
				}
			})
		}
	}
}
func TestStandingServiceRetainsPatchPresenceNumericTruncationMissingAndOriginalCauses(t *testing.T) {
	port := &standingPort{found: true, current: orders.Order{ID: "owned", Name: "original", Agent: "writer", Plan: "old", Enabled: true, CreatedMS: 1}}
	service := appstanding.New(port, port, appstanding.Host{})
	in := appstanding.EditInput{ID: "owned", Name: appstanding.TextField{Value: "changed", Present: true}, Plan: appstanding.TextField{Value: "", Present: true}, Agent: appstanding.TextField{Value: " ", Present: true}, Mode: appstanding.TextField{Value: "ask", Present: true}, MaxTrust: appstanding.TextField{Value: "L2", Present: true}, BriefingMin: appstanding.TextField{Value: "alert", Present: true}, Assure: appstanding.NumberField{Value: 3.9, Present: true}, Cooldown: appstanding.NumberField{Value: 901.9, Present: true}}
	out, err := service.Edit(context.Background(), in)
	if err != nil || !out.Updated || out.Order == nil || out.Order.Name != "changed" || out.Order.Plan != "" || out.Order.Agent != "" || out.Order.Initiative.Mode != orders.InitiativeAsk || out.Order.Initiative.MaxTrust != "L2" || out.Order.BriefingMin != "alert" || out.Order.Assure != 3 || out.Order.CooldownSec != 901 || out.Order.ID != "owned" || out.Order.CreatedMS != 1 || !out.Order.Enabled {
		t.Fatal(out, err)
	}
	port.found = false
	out, err = service.Edit(context.Background(), appstanding.EditInput{ID: "missing"})
	raw, _ := json.Marshal(out)
	if err != nil || string(raw) != `{"updated":false}` {
		t.Fatal(out, err, string(raw))
	}
	_, err = service.SetEnabled(context.Background(), appstanding.EnableInput{ID: "missing", Enabled: true})
	if err == nil || err.Error() != "unknown standing order: missing" {
		t.Fatal(err)
	}
	cause := errors.New("owned writer cause")
	port.cause = cause
	for _, operation := range []string{"add", "edit", "enable", "remove"} {
		switch operation {
		case "add":
			out, err := service.Add(context.Background(), appstanding.AddInput{})
			if err != cause || !reflect.DeepEqual(out, appstanding.OrderOutput{}) {
				t.Fatal(out, err)
			}
		case "edit":
			out, err := service.Edit(context.Background(), appstanding.EditInput{})
			if err != cause || !reflect.DeepEqual(out, appstanding.EditOutput{}) {
				t.Fatal(out, err)
			}
		case "enable":
			out, err := service.SetEnabled(context.Background(), appstanding.EnableInput{})
			if err != cause || !reflect.DeepEqual(out, appstanding.OrderOutput{}) {
				t.Fatal(out, err)
			}
		case "remove":
			out, err := service.Remove(context.Background(), appstanding.RemoveInput{ID: "missing"})
			if err != cause || !reflect.DeepEqual(out, appstanding.RemoveOutput{}) {
				t.Fatal(out, err)
			}
		}
	}
}
func TestStandingServiceUsesActualRuntimeFacadeStoreAndLifecycleJournal(t *testing.T) {
	dir := t.TempDir()
	provider := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	service := appstanding.New(k.Standing(), k, appstanding.Host{})
	added, err := service.Add(context.Background(), appstanding.AddInput{Order: orders.Order{Name: "owned", Enabled: false, ID: "client-id", CreatedMS: 1, UpdatedMS: 1, Triggers: []orders.Trigger{{Type: orders.TriggerEvent, Subject: "fixture"}}, CooldownSec: 60}})
	if err != nil || added.Order.ID == "client-id" || !added.Order.Enabled || added.Order.CreatedMS == 1 || added.Order.FrequencyWarning == "" {
		t.Fatal(added, err)
	}
	id := added.Order.ID
	edited, err := service.Edit(context.Background(), appstanding.EditInput{ID: id, Name: appstanding.TextField{Value: "edited", Present: true}})
	if err != nil || !edited.Updated || edited.Order.Name != "edited" || edited.Order.ID != id || edited.Order.CreatedMS != added.Order.CreatedMS {
		t.Fatal(edited, err)
	}
	paused, err := service.SetEnabled(context.Background(), appstanding.EnableInput{ID: id, Enabled: false})
	if err != nil || paused.Order.Enabled {
		t.Fatal(paused, err)
	}
	removed, err := service.Remove(context.Background(), appstanding.RemoveInput{ID: id})
	if err != nil || !removed.Removed || removed.ID != id {
		t.Fatal(removed, err)
	}
	removed, err = service.Remove(context.Background(), appstanding.RemoveInput{ID: id})
	if err != nil || removed.Removed {
		t.Fatal(removed, err)
	}
	kinds := []event.Kind{}
	if err := k.Journal().Range(func(e *event.Event) error {
		if strings.HasPrefix(string(e.Kind), "standing.") {
			kinds = append(kinds, e.Kind)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kinds, []event.Kind{event.KindStandingCreated, event.KindStandingUpdated, event.KindStandingUpdated, event.KindStandingRemoved}) || provider.CallCount() != 0 {
		t.Fatal(kinds, provider.CallCount())
	}
}
