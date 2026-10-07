// SPDX-License-Identifier: MIT
package schedule_test

import (
	"context"
	"encoding/json"
	"errors"
	appschedule "github.com/agezt/agezt/kernel/app/schedule"
	"github.com/agezt/agezt/kernel/cadence"
	"reflect"
	"strings"
	"testing"
	"time"
)

type reader struct {
	rows  []cadence.Entry
	id    string
	found bool
}

func (r *reader) List() []cadence.Entry { return r.rows }
func (r *reader) Get(id string) (cadence.Entry, bool) {
	r.id = id
	if len(r.rows) == 0 {
		return cadence.Entry{}, false
	}
	return r.rows[0], r.found
}
func TestScheduleListRetainsProjectionOrderAnnotationPresenceAndBestEffort(t *testing.T) {
	rows := []cadence.Entry{{ID: "first", Intent: "owned", Agent: " writer ", Target: cadence.TargetIntent, IntervalSec: 60, Enabled: false}, {ID: "second", Target: cadence.TargetTool, Tool: "read", Payload: json.RawMessage(`{}`)}}
	store := &reader{rows: rows}
	cause := errors.New("owned annotation cause")
	host := appschedule.Host{LastFirings: func() (map[string]appschedule.LastFiring, error) {
		return map[string]appschedule.LastFiring{"first": {Status: "completed", Reason: "", FiredMS: 0}}, cause
	}, Validate: func(entry cadence.Entry) error {
		if entry.ID == "second" {
			return errors.New("blocked cause")
		}
		return nil
	}, Warning: func(entry cadence.Entry) string {
		if entry.ID == "first" {
			return "frequency warning"
		}
		return ""
	}}
	out, err := appschedule.New(store, host, nil).List(context.Background(), appschedule.ListInput{})
	if err != nil || out.Count != 2 || out.Schedules[0].ID != "first" || out.Schedules[1].ID != "second" || out.Schedules[0].TargetStatus != "ready" || out.Schedules[1].TargetStatus != "blocked" || out.Schedules[1].TargetError != "blocked cause" || out.Schedules[0].FrequencyWarning != "frequency warning" || out.Schedules[0].LastStatus == nil || *out.Schedules[0].LastStatus != "completed" || out.Schedules[0].LastReason == nil || *out.Schedules[0].LastReason != "" || out.Schedules[0].LastFiredMS == nil || *out.Schedules[0].LastFiredMS != 0 || out.Schedules[1].LastStatus != nil {
		t.Fatal(out, err)
	}
	raw, _ := json.Marshal(out)
	var root map[string]any
	_ = json.Unmarshal(raw, &root)
	first := root["schedules"].([]any)[0].(map[string]any)
	for _, key := range []string{"mode", "at_minutes", "end_minutes", "days", "tz", "model", "workflow", "system_task", "tool", "payload", "enabled", "last_run_unix", "fires", "assure", "last_reason", "last_fired_unix_ms"} {
		if _, exists := first[key]; !exists {
			t.Fatal("required empty/zero field lost", key, string(raw))
		}
	}
	store.rows = nil
	empty, err := appschedule.New(store, appschedule.Host{}, nil).List(context.Background(), appschedule.ListInput{})
	raw, _ = json.Marshal(empty)
	if err != nil || string(raw) != "{\"schedules\":[],\"count\":0}" {
		t.Fatal(empty, err, string(raw))
	}
}
func TestScheduleProjectionRetainsTargetAuthorityPayloadAndLLMBoundaries(t *testing.T) {
	cases := []struct {
		target, agent, executor, payloadText, identity string
		llm                                            bool
	}{{cadence.TargetIntent, " writer ", "agent", "task text only", "agent writer", true}, {cadence.TargetIntent, "", "llm", "task text only", "none; ad-hoc governed task", true}, {cadence.TargetWorkflow, " writer ", "workflow", "cron passes JSON workflow payload", "agent writer", true}, {cadence.TargetWorkflow, "", "workflow", "cron passes JSON workflow payload", "none; workflow is a reusable graph", true}, {cadence.TargetSystemTask, "", "daemon", "payload not accepted", "none; daemon task owns no agent soul", false}, {cadence.TargetTool, " writer ", "tool", "cron passes JSON tool payload", "agent writer tool policy", false}, {cadence.TargetTool, "", "tool", "cron passes JSON tool payload", "none; tool call owns no agent soul", false}}
	for _, item := range cases {
		entry := cadence.Entry{ID: "owned", Target: item.target, Agent: item.agent, Workflow: " graph ", Tool: " read ", SystemTask: " memory_clean ", Payload: json.RawMessage(`{}`)}
		record := appschedule.Project(entry)
		if record.Executor != item.executor || record.UsesLLM != item.llm || record.PayloadContract != item.payloadText || record.IdentityOwner != item.identity || record.LLMBoundary == "" || record.ExecutionAuthority == "" || record.ExecutionContract == "" || !reflect.DeepEqual(record.Payload, entry.Payload) {
			t.Fatal(item, record)
		}
	}
	for _, target := range []string{cadence.TargetTool, cadence.TargetWorkflow} {
		for _, payload := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(" ")} {
			text := appschedule.PayloadContract(cadence.Entry{Target: target, Payload: payload})
			if !strings.Contains(text, "no ") {
				t.Fatal(target, string(payload), text)
			}
		}
	}
}
func TestScheduleForecastRetainsNotFoundAndPresentFalseZeroFields(t *testing.T) {
	now := time.Unix(1000, 0)
	entry := cadence.Entry{ID: "canonical", Mode: cadence.ModeInterval, IntervalSec: 60, NextRunUnix: 1060, Enabled: false}
	store := &reader{rows: []cadence.Entry{entry}, found: true}
	service := appschedule.New(store, appschedule.Host{}, func() time.Time { return now })
	out, err := service.Test(context.Background(), appschedule.TestInput{ID: "alias", Count: 3})
	want := entry.Forecast(now, 3)
	if err != nil || !out.Found || out.ID == nil || *out.ID != "canonical" || out.Enabled == nil || *out.Enabled || out.Mode == nil || *out.Mode != "" || out.Cadence == nil || *out.Cadence != entry.Cadence() || out.Forecasts == nil || out.Count == nil || *out.Count != len(want) || store.id != "alias" {
		t.Fatal(out, err)
	}
	for i, row := range *out.Forecasts {
		if row.Unix != want[i] {
			t.Fatal(row, want)
		}
	}
	raw, _ := json.Marshal(out)
	var root map[string]any
	_ = json.Unmarshal(raw, &root)
	for _, key := range []string{"id", "mode", "cadence", "enabled", "forecasts", "count"} {
		if _, exists := root[key]; !exists {
			t.Fatal("found field omitted", key, string(raw))
		}
	}
	store.found = false
	out, err = service.Test(context.Background(), appschedule.TestInput{ID: "missing"})
	raw, _ = json.Marshal(out)
	if err != nil || string(raw) != "{\"found\":false}" {
		t.Fatal(out, err, string(raw))
	}
}
func TestScheduleSystemTasksRetainsActualCatalogAndIndependentSlices(t *testing.T) {
	service := appschedule.New(&reader{}, appschedule.Host{}, nil)
	out, err := service.SystemTasks(context.Background(), appschedule.SystemTasksInput{})
	if err != nil || out.Count != len(cadence.SystemTasks()) || !reflect.DeepEqual(out.SystemTasks, cadence.SystemTasks()) || !reflect.DeepEqual(out.SystemTaskInfo, cadence.SystemTaskInfos()) {
		t.Fatal(out, err)
	}
	out.SystemTasks[0] = "mutated"
	out.SystemTaskInfo[0].Name = "mutated"
	fresh, _ := service.SystemTasks(context.Background(), appschedule.SystemTasksInput{})
	if fresh.SystemTasks[0] == "mutated" || fresh.SystemTaskInfo[0].Name == "mutated" {
		t.Fatal("catalog slices shared")
	}
}
