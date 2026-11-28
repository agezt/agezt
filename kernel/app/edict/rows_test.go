// SPDX-License-Identifier: MIT

package edict

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// edictFamily gathers every operation the edict family declares: the policy
// reads, the policy writes, the overlay's two and the two decision reads.
func edictFamily(t *testing.T) []app.Operation {
	t.Helper()
	var ops []app.Operation
	reads, err := Operations(func(context.Context) *Service { return New(engine()) })
	if err != nil {
		t.Fatal(err)
	}
	writes, err := WriteOperations(func(context.Context) *Writes { return NewWrites(&fakeWriteEngine{}, (&recorder{}).publish) })
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := OverlayOperations(func(context.Context) *Overlay { return NewOverlay(policyJournal(), nil, nil) })
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := DecisionOperations(func(context.Context) *Decisions { return NewDecisions(decisions(), time.Now) })
	if err != nil {
		t.Fatal(err)
	}
	return append(append(append(ops, reads...), writes...), append(overlay, decisions...)...)
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

// TestTheEdictRowsCarryTheFamiliesOwnRules says the edict family reads its
// arguments from the contract's own rows: the schema the door declares for an
// operation IS the schema derived from that row — the family keeps no schema
// of its own — and a mistyped argument is refused at the door, before the
// service runs.
func TestTheEdictRowsCarryTheFamiliesOwnRules(t *testing.T) {
	ops := edictFamily(t)
	if len(ops) != 11 {
		t.Fatalf("the family declares %d operations, want 11", len(ops))
	}
	rows := map[string]any{}
	for _, binding := range opapi.RequestRows {
		rows[binding.Operation] = binding.Row
	}
	rowTypes := map[string]bool{}
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
		rowTypes[reflect.TypeOf(row).String()] = true
	}
	// Nine rows serve the family's eleven operations: the overlay's two
	// operations share the one row they both take.
	if len(rowTypes) != 9 {
		t.Fatalf("the family's eleven operations read %d rows, want 9", len(rowTypes))
	}
	if !sameSchema(doorOf(t, ops, opapi.EdictOverlay), doorOf(t, ops, opapi.EdictCompact)) {
		t.Fatal("the overlay's two operations must share their row")
	}
	// The door refuses a mistyped argument: the family's strictness lives in
	// its rows, not in a second reading of the body beside them.
	for _, w := range []struct {
		name string
		raw  string
	}{
		{opapi.EdictShow, `{"tenant":3}`},
		{opapi.EdictShow, `{"tenant":null}`},
		{opapi.EdictTest, `{"capability":3}`},
		{opapi.EdictTest, `{"input":42}`},
		{opapi.EdictDenyAdd, `{"rule":3}`},
		{opapi.EdictDenyRemove, `{"name":3}`},
		{opapi.EdictSetLevel, `{"level":3}`},
		{opapi.EdictSetMode, `{"mode":3}`},
		{opapi.EdictOverlay, `{"tenant":3}`},
		{opapi.EdictCompact, `{"tenant":3}`},
		{opapi.EdictLog, `{"limit":"5"}`},
		{opapi.EdictLog, `{"denied":"yes"}`},
		{opapi.EdictStats, `{"since_ms":"x"}`},
	} {
		if err := schema.ValidateJSON(doorOf(t, ops, w.name), json.RawMessage(w.raw)); err == nil {
			t.Fatalf("%s admitted %s", w.name, w.raw)
		}
	}
	// The page is the family's own: the default of twenty, the floor of one
	// and the ceiling of a thousand, with the window truncated toward zero.
	var many fakeJournal
	for i := range 1_005 {
		many.events = append(many.events, &event.Event{Seq: int64(i + 1), TSUnixMS: int64(i + 1), Kind: event.KindPolicyDecision, Payload: json.RawMessage(`{"allow":true}`)})
	}
	page := NewDecisions(many, time.Now)
	for raw, want := range map[string]int{`{}`: 20, `{"limit":25}`: 25, `{"limit":2000}`: 1_000, `{"limit":0}`: 1} {
		out, err := page.Log(context.Background(), decode[opapi.EdictDecisionLogRequest](t, raw))
		if err != nil || out.Count != want {
			t.Fatalf("%s counted %d rows, want %d", raw, out.Count, want)
		}
	}
	d := NewDecisions(decisions(), func() time.Time { return clock })
	window, _ := d.Log(context.Background(), decode[opapi.EdictDecisionLogRequest](t, `{"since_ms":2500.9}`))
	if window.Count != 3 {
		t.Fatalf("the window is the daemon clock minus a truncated since_ms, got %d rows", window.Count)
	}
}
