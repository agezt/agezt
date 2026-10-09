// SPDX-License-Identifier: MIT

package budget

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type fakeGovernor struct {
	snap Snapshot
	sets []int64
}

func (f *fakeGovernor) Snapshot() Snapshot {
	snap := f.snap
	snap.PerTask = append([]TaskSnapshot(nil), f.snap.PerTask...)
	return snap
}

func (f *fakeGovernor) SetDailyCeiling(mc int64) {
	f.sets = append(f.sets, mc)
	if mc < 0 {
		mc = 0
	}
	f.snap.CeilingMicrocents = mc
}

func setRequest(t *testing.T, raw string) SetRequest {
	t.Helper()
	var in SetRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func encode(t *testing.T, out Output) string {
	t.Helper()
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestGet(t *testing.T) {
	ctx := context.Background()
	if _, err := New(nil).Get(ctx, GetRequest{}); err == nil || err.Error() != "budget: daemon's provider is not a governor (likely a test or future provider variant); no budget state to report" {
		t.Fatal("no governor", err)
	}
	gov := &fakeGovernor{snap: Snapshot{UTCDate: "2026-05-29", SpentMicrocents: 7, CeilingMicrocents: 1_000_000_000, StrictPricing: true}}
	out, err := New(gov).Get(ctx, GetRequest{})
	if got := encode(t, out); err != nil || got != `{"utc_date":"2026-05-29","spent_mc":7,"ceiling_mc":1000000000,"per_task":[],"strict_pricing":true}` {
		t.Fatal("no per-task caps is an empty array", got, err)
	}
	gov.snap.PerTask = []TaskSnapshot{{TaskType: "plan", SpentMicrocents: 3, CapMicrocents: 100}, {TaskType: "code", SpentMicrocents: 0, CapMicrocents: 500}, {TaskType: "chat", SpentMicrocents: 9, CapMicrocents: 0}}
	out, err = New(gov).Get(ctx, GetRequest{})
	if got := encode(t, out); err != nil || got != `{"utc_date":"2026-05-29","spent_mc":7,"ceiling_mc":1000000000,"per_task":[{"task_type":"chat","spent_mc":9,"ceiling_mc":0},{"task_type":"code","spent_mc":0,"ceiling_mc":500},{"task_type":"plan","spent_mc":3,"ceiling_mc":100}],"strict_pricing":true}` {
		t.Fatal("per-task rows sorted by task type", got, err)
	}
	if len(gov.sets) != 0 {
		t.Fatal("a read never sets", gov.sets)
	}
}

func TestSetRefusals(t *testing.T) {
	ctx := context.Background()
	if _, err := New(nil).Set(ctx, setRequest(t, `{}`)); err == nil || err.Error() != "budget_set: daemon's provider is not a governor; cannot adjust the ceiling" {
		t.Fatal("no governor is refused before the argument", err)
	}
	gov := &fakeGovernor{snap: Snapshot{CeilingMicrocents: 11}}
	for raw, want := range map[string]string{
		`{}`:                                    "args.ceiling_mc required (microcents; 0 = unlimited)",
		`{"ceiling_mc":null}`:                   "args.ceiling_mc: unexpected type <nil>",
		`{"ceiling_mc":true}`:                   "args.ceiling_mc: unexpected type bool",
		`{"ceiling_mc":{}}`:                     "args.ceiling_mc: unexpected type map[string]interface {}",
		`{"ceiling_mc":[1]}`:                    "args.ceiling_mc: unexpected type []interface {}",
		`{"ceiling_mc":1.5}`:                    "args.ceiling_mc: must be a whole number, got 1.5",
		`{"ceiling_mc":1e300}`:                  "args.ceiling_mc: must be a whole number, got 1e+300",
		`{"ceiling_mc":"abc"}`:                  `args.ceiling_mc: not an integer: "abc"`,
		`{"ceiling_mc":" 4.0 "}`:                `args.ceiling_mc: not an integer: " 4.0 "`,
		`{"ceiling_mc":""}`:                     `args.ceiling_mc: not an integer: ""`,
		`{"ceiling_mc":"99999999999999999999"}`: `args.ceiling_mc: not an integer: "99999999999999999999"`,
	} {
		if _, err := New(gov).Set(ctx, setRequest(t, raw)); err == nil || err.Error() != want {
			t.Fatalf("%s: %v, want %q", raw, err, want)
		}
	}
	if len(gov.sets) != 0 || gov.snap.CeilingMicrocents != 11 {
		t.Fatal("a refusal never sets", gov.sets)
	}
}

func TestSet(t *testing.T) {
	ctx := context.Background()
	gov := &fakeGovernor{snap: Snapshot{UTCDate: "2026-05-29", SpentMicrocents: 5, CeilingMicrocents: 1, PerTask: []TaskSnapshot{{TaskType: "plan", CapMicrocents: 2}, {TaskType: "code", CapMicrocents: 3}}}}
	out, err := New(gov).Set(ctx, setRequest(t, `{"ceiling_mc":2500000000,"tenant":"x"}`))
	if got := encode(t, out); err != nil || got != `{"utc_date":"2026-05-29","spent_mc":5,"ceiling_mc":2500000000,"per_task":[{"task_type":"code","spent_mc":0,"ceiling_mc":3},{"task_type":"plan","spent_mc":0,"ceiling_mc":2}],"strict_pricing":false}` {
		t.Fatal("the post-set snapshot is returned", got, err)
	}
	for raw, want := range map[string]int64{
		`{"ceiling_mc":" 750000000 "}`: 750_000_000,
		`{"ceiling_mc":"-3"}`:          -3,
		`{"ceiling_mc":-4}`:            -4,
		`{"ceiling_mc":0}`:             0,
		`{"ceiling_mc":4.0}`:           4,
		`{"ceiling_mc":-0}`:            0,
	} {
		gov.sets = nil
		if _, err := New(gov).Set(ctx, setRequest(t, raw)); err != nil || len(gov.sets) != 1 || gov.sets[0] != want {
			t.Fatalf("%s: set %v, want %d (%v)", raw, gov.sets, want, err)
		}
	}
	if out, _ := New(gov).Set(ctx, setRequest(t, `{"ceiling_mc":-9}`)); out.CeilingMC != 0 {
		t.Fatal("the snapshot reports the governor's clamp", out.CeilingMC)
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("provider is required")
	}
	ops, err := Operations(func(context.Context) *Service { return New(nil) })
	if err != nil || len(ops) != 2 {
		t.Fatal(len(ops), err)
	}
	out, err := schema.FromType(reflect.TypeFor[Output](), false)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []struct {
		name, method, path string
		read               bool
		input              reflect.Type
	}{
		{"budget", "GET", "/api/budget", true, reflect.TypeFor[GetRequest]()},
		{"budget_set", "", "", false, reflect.TypeFor[SetRequest]()},
	} {
		s := ops[i].Spec()
		if s.Name != want.name || s.ReadOnly != want.read || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.HTTP.Method != want.method || s.HTTP.Path != want.path || s.Input != want.input || s.Output != reflect.TypeFor[Output]() || string(s.OutputSchema) != string(out) {
			t.Fatalf("%s spec: %+v", want.name, s)
		}
	}
}
