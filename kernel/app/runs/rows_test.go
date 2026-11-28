// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// runsFamily gathers every operation the runs family declares: the governed
// run, its two reads, the plan's own run and reads, and the plan authoring.
func runsFamily(t *testing.T) []app.Operation {
	t.Helper()
	var ops []app.Operation
	run, err := RunOperations(func(context.Context) *RunService { return nil })
	if err != nil {
		t.Fatal(err)
	}
	reads, err := Operations(func(context.Context) *Service { return nil })
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanRunOperations(func(context.Context) *PlanRunner { return nil })
	if err != nil {
		t.Fatal(err)
	}
	plans, err := PlanOperations(func(context.Context) *Plans { return nil })
	if err != nil {
		t.Fatal(err)
	}
	authoring, err := PlanAuthoringOperations(func(context.Context) *PlanAuthoring { return nil })
	if err != nil {
		t.Fatal(err)
	}
	ops = append(ops, run...)
	ops = append(ops, reads...)
	ops = append(ops, plan...)
	ops = append(ops, plans...)
	return append(ops, authoring...)
}

// doorOf returns the schema the door declares for one operation.
func doorOf(t *testing.T, ops []app.Operation, name string) json.RawMessage {
	t.Helper()
	for _, op := range ops {
		if op.Spec().Name == name {
			return op.Spec().InputSchema
		}
	}
	t.Fatalf("the family declares no %s operation", name)
	return nil
}

// sameSchema reports whether two schemas say the same thing, whatever order
// their keys are written in.
func sameSchema(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

// TestTheRunsRowsCarryTheFamiliesOwnRules says the runs family reads its
// arguments from the contract's own rows: the schema the door declares for an
// operation IS the schema derived from that row — the family keeps no schema
// of its own — and the family's own rules stay in the family.
func TestTheRunsRowsCarryTheFamiliesOwnRules(t *testing.T) {
	ops := runsFamily(t)
	if len(ops) != 8 {
		t.Fatalf("the family declares %d operations, want 8", len(ops))
	}
	rows := map[string]any{}
	for _, binding := range opapi.RequestRows {
		rows[binding.Operation] = binding.Row
	}
	for _, op := range ops {
		spec := op.Spec()
		row, ok := rows[spec.Name]
		if !ok {
			t.Fatalf("the contract names no request row for %s", spec.Name)
		}
		derived, err := schema.FromType(reflect.TypeOf(row), true)
		if err != nil {
			t.Fatal(spec.Name, err)
		}
		if !sameSchema(spec.InputSchema, derived) {
			t.Fatalf("%s declares a schema of its own:\n declared: %s\n derived:  %s", spec.Name, spec.InputSchema, derived)
		}
	}
	// The door refuses the family's mistyped arguments: the strictness lives
	// in the rows, not in a second reading of the body beside them.
	for _, w := range []struct {
		name string
		raw  string
	}{
		{opapi.Run, `{"intent":7}`},
		{opapi.Run, `{"intent":"x","images":"a"}`},
		{opapi.Run, `{"intent":"x","images":[7]}`},
		{opapi.Run, `{"intent":"x","tools":[7]}`},
		{opapi.Run, `{"intent":"x","max_cost":"5"}`},
		{opapi.Run, `{"intent":"x","dry_run":7}`},
		{opapi.Run, `{"intent":"x","auto_approve_caps":[7]}`},
		{opapi.Run, `{"intent":"x","prompt_injection_trust":7}`},
		{opapi.RunsList, `{"limit":"5"}`},
		{opapi.RunsList, `{"cursor":3}`},
		{opapi.RunsStats, `{"since_ms":"x"}`},
		{opapi.Plan, `{"plan_json":7}`},
		{opapi.PlanHistory, `{"status":3}`},
		{opapi.PlanGenerate, `{"intent":7}`},
		{opapi.PlanRefine, `{"feedback":7}`},
	} {
		if err := schema.ValidateJSON(doorOf(t, ops, w.name), json.RawMessage(w.raw)); err == nil {
			t.Fatalf("%s admitted %s", w.name, w.raw)
		}
	}
	// The family's own rules stay in the family: the page is twenty rows,
	// never fewer than one, never more than a thousand; the window is
	// truncated toward zero; the auto-approve grant is split on commas and
	// spaces; the trust flag reads its own spellings.
	d := New(fixed(Run{CorrelationID: "a"}, Run{CorrelationID: "b"}), clock)
	for raw, want := range map[string]int{`{}`: 2, `{"limit":1}`: 1, `{"limit":0}`: 1, `{"limit":-4}`: 1} {
		out, err := d.List(context.Background(), decode[opapi.RunListRequest](t, raw))
		if err != nil || out.Count != want {
			t.Fatalf("%s counted %d rows, want %d", raw, out.Count, want)
		}
	}
	caps := approveCaps(decode[opapi.StrictTextList](t, `"a, b c"`))
	if !reflect.DeepEqual(caps, map[string]bool{"a": true, "b": true, "c": true}) {
		t.Fatalf("the grant splits on commas and spaces: %v", caps)
	}
	if !truthy(decode[opapi.BoolFlag](t, `"on"`)) {
		t.Fatal("the trust flag reads its own spelling")
	}
}
