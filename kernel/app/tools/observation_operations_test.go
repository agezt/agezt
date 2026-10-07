// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	"reflect"
	"strings"
	"testing"
)

func TestToolObservationTypedAdmissionCodecsClockAndNoAudit(t *testing.T) {
	if _, err := ObservationOperations(nil, nil); err == nil {
		t.Fatal("missing provider admitted")
	}
	port := &observationJournal{events: observationFixture()}
	providers, clock := 0, 0
	ops, err := ObservationOperations(func(context.Context) *Observations { providers++; return NewObservations(port) }, func() int64 { clock++; return 1000 })
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	audit := &inventoryForbiddenAudit{}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: inventoryAuth{opapi.Tenant}, Router: inventoryRoute{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	caller := opapi.Caller{Tenant: "acme"}
	for _, tc := range []struct{ name, raw, error string }{{"tool_log", `{"errors":"true","tool":true}`, "args.errors must be a boolean"}, {"tool_log", `{"errors":null}`, "args.errors must be a boolean"}, {"tool_log", `{"tool":null}`, "args.tool must be a string"}, {"tool_stats", `{"errors":"true","tool":true}`, "args.tool must be a string"}} {
		_, err := d.Dispatch(context.Background(), caller, tc.name, json.RawMessage(tc.raw), nil)
		if err == nil || err.Error() != tc.error {
			t.Fatal(tc, err)
		}
	}
	if providers != 0 || port.reads != 0 || clock != 0 {
		t.Fatal(providers, port.reads, clock)
	}
	for _, tc := range []struct {
		name, raw string
		count     int
	}{{"tool_log", `{}`, 6}, {"tool_log", `{"limit":0}`, 1}, {"tool_log", `{"limit":1.9}`, 1}, {"tool_log", `{"limit":"1"}`, 6}, {"tool_log", `{"errors":true}`, 2}, {"tool_log", `{"tool":" foo "}`, 0}, {"tool_log", `{"slow_ms":"20"}`, 6}, {"tool_stats", `{"errors":null,"limit":false,"slow_ms":null,"cursor":true}`, 6}} {
		out, err := d.Dispatch(context.Background(), caller, tc.name, json.RawMessage(tc.raw), nil)
		if err != nil {
			t.Fatal(tc, err)
		}
		if log, ok := out.(LogOutput); ok {
			if log.Count != tc.count {
				t.Fatal(tc, out)
			}
		} else if stats := out.(StatsOutput); stats.Total != tc.count {
			t.Fatal(tc, out)
		}
	}
	out, err := d.Dispatch(context.Background(), caller, "tool_log", json.RawMessage(`{"since_ms":960,"unused":true}`), nil)
	if err != nil || out.(LogOutput).Count != 5 || clock != 1 {
		t.Fatal(out, err, clock)
	}
	statsOut, err := d.Dispatch(context.Background(), caller, "tool_stats", json.RawMessage(`{"since_ms":-10}`), nil)
	if err != nil || statsOut.(StatsOutput).Total != 6 || statsOut.(StatsOutput).WindowMS != -10 || clock != 1 {
		t.Fatal(statsOut, err, clock)
	}
	beforeProviders, beforeReads := providers, port.reads
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, name := range []string{"tool_log", "tool_stats"} {
		if _, err := d.Dispatch(ctx, caller, name, json.RawMessage(`{}`), nil); err != context.Canceled {
			t.Fatal(name, err)
		}
	}
	if _, err := d.Dispatch(context.Background(), opapi.Caller{Tenant: "other"}, "tool_log", json.RawMessage(`{}`), nil); err == nil {
		t.Fatal("tenant identity mismatch admitted")
	}
	if providers != beforeProviders || port.reads != beforeReads || audit.calls != 0 {
		t.Fatal(providers, port.reads, audit.calls)
	}
	for _, operation := range ops {
		spec := operation.Spec()
		if !spec.ReadOnly || spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant || spec.Stream != opapi.StreamNone || spec.Input != reflect.TypeFor[ObservationRequest]() || !spec.AllowUnknownInput {
			t.Fatal(spec)
		}
		output, err := d.Dispatch(context.Background(), caller, spec.Name, json.RawMessage(`{}`), nil)
		encoded, _ := json.Marshal(output)
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.ValidateJSON(spec.OutputSchema, encoded); err != nil {
			t.Fatal(spec.Name, err)
		}
	}
	for _, tc := range []struct {
		raw   string
		limit int
	}{{"", 20}, {"null", 20}, {`"1"`, 20}, {"true", 20}, {"0", 1}, {"-10", 1}, {"1.9", 1}, {"2000", 1000}} {
		if got := observationLimit(json.RawMessage(tc.raw)); got != tc.limit {
			t.Fatal(tc, got)
		}
	}
}
func TestToolObservationTypedSchemasRequireZeroFalseNullAndOptionalAverage(t *testing.T) {
	ops, err := ObservationOperations(func(context.Context) *Observations { return NewObservations(&observationJournal{}) }, nil)
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]json.RawMessage{}
	for _, op := range ops {
		declared[op.Spec().Name] = op.Spec().OutputSchema
	}
	raw, _ := json.Marshal(LogOutput{Invocations: []LogItem{{}}, Count: 1})
	var wire map[string]any
	_ = json.Unmarshal(raw, &wire)
	row := wire["invocations"].([]any)[0].(map[string]any)
	if len(row) != 14 || row["error"] != false || row["directive_like"] != false || row["directive_matches"] != nil {
		t.Fatal(row)
	}
	for _, field := range []string{"actor", "correlation_id", "tool", "call_id", "input", "output", "error", "duration_ms", "observation_trust", "observation_source", "directive_like", "directive_matches", "seq", "ts_unix_ms"} {
		value, ok := row[field]
		if !ok {
			t.Fatal(field, row)
		}
		delete(row, field)
		encoded, _ := json.Marshal(wire)
		if err := schema.ValidateJSON(declared["tool_log"], encoded); err == nil {
			t.Fatal("missing required row field admitted", field)
		}
		row[field] = value
	}
	for _, field := range []string{"invocations", "count", "next_cursor"} {
		value := wire[field]
		delete(wire, field)
		encoded, _ := json.Marshal(wire)
		if err := schema.ValidateJSON(declared["tool_log"], encoded); err == nil {
			t.Fatal("missing required log root admitted", field)
		}
		wire[field] = value
	}
	zero := int64(0)
	out := StatsOutput{ByTool: map[string]ToolSummary{"sampled": {AvgMS: &zero}, "unsampled": {}}, ErrorsByMessage: map[string]int{}}
	encoded, _ := json.Marshal(out)
	if err := schema.ValidateJSON(declared["tool_stats"], encoded); err != nil {
		t.Fatal(err)
	}
	body := observationWire(t, out)
	byTool := body["by_tool"].(map[string]any)
	if byTool["sampled"].(map[string]any)["avg_ms"] != float64(0) {
		t.Fatal(body)
	}
	if _, ok := byTool["unsampled"].(map[string]any)["avg_ms"]; ok {
		t.Fatal(body)
	}
	for _, field := range []string{"total", "errored", "error_rate", "by_tool", "tools", "window_ms", "errors_by_message", "duration_ms"} {
		value := body[field]
		delete(body, field)
		raw, _ := json.Marshal(body)
		if err := schema.ValidateJSON(declared["tool_stats"], raw); err == nil {
			t.Fatal("missing stats root admitted", field)
		}
		body[field] = value
	}
	duration := body["duration_ms"].(map[string]any)
	for _, field := range []string{"count", "avg", "min", "max", "p50", "p95"} {
		value := duration[field]
		delete(duration, field)
		raw, _ := json.Marshal(body)
		if err := schema.ValidateJSON(declared["tool_stats"], raw); err == nil {
			t.Fatal("missing duration field admitted", field)
		}
		duration[field] = value
	}
	item := byTool["unsampled"].(map[string]any)
	for _, field := range []string{"calls", "errors"} {
		value := item[field]
		delete(item, field)
		raw, _ := json.Marshal(body)
		if err := schema.ValidateJSON(declared["tool_stats"], raw); err == nil {
			t.Fatal("missing by-tool field admitted", field)
		}
		item[field] = value
	}
	if strings.Contains(string(encoded), `"avg_ms":null`) {
		t.Fatal(string(encoded))
	}
}
