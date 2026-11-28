// SPDX-License-Identifier: MIT

package opapi

// The request rows and the arguments they carry.
//
// A request row is one declaration with three readers — the registry derives
// the schema from it, the application layer decodes it, the console builds its
// arguments from it — so the row's field names are the only spelling of them.

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/skillapi"
)

// TestTheRequestRowsAreBoundToTheirOperations says every row is bound to an
// operation, and that the binding names the row the operation owns: a row
// that drifted from its operation would be a promise the operation never
// made.
func TestTheRequestRowsAreBoundToTheirOperations(t *testing.T) {
	if len(RequestRows) == 0 {
		t.Fatal("the contract names no request row")
	}
	seen := map[string]bool{}
	for _, binding := range RequestRows {
		if binding.Operation == "" {
			t.Errorf("a request row is bound to no operation: %T", binding.Row)
			continue
		}
		if seen[binding.Operation] {
			t.Errorf("the %s operation is bound to two request rows", binding.Operation)
		}
		seen[binding.Operation] = true
		shape := reflect.TypeOf(binding.Row)
		if shape == nil || shape.Kind() != reflect.Struct {
			t.Errorf("the %s request row is not a row: %T", binding.Operation, binding.Row)
			continue
		}
		if shape.NumField() == 0 {
			// A row that names no field says "this operation takes no
			// arguments" — and the catalogue must agree: an operation the
			// console forwards arguments to has a row that names them.
			for _, route := range ConsoleRoutes() {
				if route.Op == binding.Operation && len(route.Args) > 0 {
					t.Errorf("the %s request row names no field, but the %s route forwards %v",
						binding.Operation, route.Path, route.Args)
				}
			}
		}
		row, ok := requestRowOf(binding.Operation)
		if !ok {
			t.Errorf("the %s operation's row is not reachable", binding.Operation)
			continue
		}
		if reflect.TypeOf(row) != shape {
			t.Errorf("the %s operation's row is %T, not %T", binding.Operation, row, binding.Row)
		}
	}
}

// requestRowOf returns the request row bound to one operation.
func requestRowOf(operation string) (any, bool) {
	for _, binding := range RequestRows {
		if binding.Operation == operation {
			return binding.Row, true
		}
	}
	return nil, false
}

// TestArgsReadsTheRowsOwnFields says the arguments are the row's fields, by
// the row's own names — and that an optional field holding its zero value is
// left out, the way the wire has always carried it.
func TestArgsReadsTheRowsOwnFields(t *testing.T) {
	args := Args(FileMkdirRequest{Path: "a/b", Parents: true})
	if args["path"] != "a/b" || args["parents"] != true {
		t.Errorf("the mkdir arguments are %v", args)
	}
	if len(args) != 2 {
		t.Errorf("the mkdir arguments name more than the row's fields: %v", args)
	}
	args = Args(FileMkdirRequest{Path: "a/b"})
	if args["path"] != "a/b" {
		t.Errorf("the mkdir arguments are %v", args)
	}
	if _, present := args["parents"]; present {
		t.Errorf("an optional field holding its zero value must be left out: %v", args)
	}
	args = Args(SkillRestoreRequest{ID: "s-1", Status: "paused"})
	if args["id"] != "s-1" || args["status"] != skillapi.Status("paused") {
		t.Errorf("the skill restore arguments are %v", args)
	}
	if _, present := args["reason"]; present {
		t.Errorf("an empty reason must be left out: %v", args)
	}
}

// TestArgsNamesTheRowsWireFields says the arguments carry the row's WIRE
// names — the names the daemon's door reads — and not the Go field names.
func TestArgsNamesTheRowsWireFields(t *testing.T) {
	args := Args(FileRenameRequest{From: "a", To: "b"})
	if args["from"] != "a" || args["to"] != "b" {
		t.Errorf("the rename arguments are %v", args)
	}
	args = Args(FileRestoreRequest{ID: "c-1"})
	if args["id"] != "c-1" {
		t.Errorf("the restore arguments are %v", args)
	}
}

// TestRequiredTextRefusesABlankField says a blank text field is refused with
// the daemon's own wording.
func TestRequiredTextRefusesABlankField(t *testing.T) {
	if _, err := RequiredText("  ", "name"); err == nil || err.Error() != "args.name required" {
		t.Errorf("a blank name must be refused: %v", err)
	}
	if _, err := RequiredText("", "name"); err == nil {
		t.Error("an empty name must be refused")
	}
	value, err := RequiredText(" owned ", "name")
	if err != nil || value != " owned " {
		t.Errorf("a named field passes through: %q %v", value, err)
	}
}

// TestBoolFlagValueAcceptsTheStringsAQueryCanCarry says a flag reads as a
// boolean, or as the string a query string carries for one — and that a typo
// is refused rather than read as false.
func TestBoolFlagValueAcceptsTheStringsAQueryCanCarry(t *testing.T) {
	for _, in := range []any{true, "true", "TRUE", " yes ", "1"} {
		flag := boolFlagOf(t, in)
		value, err := BoolFlagValue(flag, "with_runs")
		if err != nil || !value {
			t.Errorf("BoolFlagValue(%#v) = (%v, %v), want (true, nil)", in, value, err)
		}
	}
	for _, in := range []any{false, "false", "FALSE", " no ", "0"} {
		flag := boolFlagOf(t, in)
		value, err := BoolFlagValue(flag, "with_runs")
		if err != nil || value {
			t.Errorf("BoolFlagValue(%#v) = (%v, %v), want (false, nil)", in, value, err)
		}
	}
}

// TestBoolFlagValueDistinguishesAbsentFromFalse says a flag that was not sent
// reads as false without being refused.
func TestBoolFlagValueDistinguishesAbsentFromFalse(t *testing.T) {
	value, err := BoolFlagValue(BoolFlag{}, "with_runs")
	if err != nil || value {
		t.Errorf("an absent flag = (%v, %v), want (false, nil)", value, err)
	}
}

// TestBoolFlagValueRejectsATypoRatherThanReadingItAsFalse says a typo is
// refused: silently treating "ture" as false is how a flag stops working
// without anyone noticing.
func TestBoolFlagValueRejectsATypoRatherThanReadingItAsFalse(t *testing.T) {
	for _, in := range []any{"ture", "", 1.0, []any{true}} {
		flag := boolFlagOf(t, in)
		if _, err := BoolFlagValue(flag, "with_runs"); err == nil {
			t.Errorf("BoolFlagValue(%#v) accepted a bad value", in)
		}
	}
}

// TestBoolFlagOrFalseReadsTheLenientWay says the lenient reading is the one
// the "enabled" flag has always had: true, "true" and "1" are on, anything
// else is off.
func TestBoolFlagOrFalseReadsTheLenientWay(t *testing.T) {
	for _, in := range []any{true, "true", "TRUE", "1"} {
		if !BoolFlagOrFalse(boolFlagOf(t, in)) {
			t.Errorf("BoolFlagOrFalse(%#v) must read as true", in)
		}
	}
	for _, in := range []any{false, "false", "0", "ture", 1.0} {
		if BoolFlagOrFalse(boolFlagOf(t, in)) {
			t.Errorf("BoolFlagOrFalse(%#v) must read as false", in)
		}
	}
	if BoolFlagOrFalse(BoolFlag{}) {
		t.Error("an absent flag must read as false")
	}
}

// TestTheFlagRidesTheWireAsItsBoolean says the flag's wrapper names its
// type, but the wire carries the boolean.
func TestTheFlagRidesTheWireAsItsBoolean(t *testing.T) {
	args := Args(WorkflowListRequest{WithRuns: boolFlagOf(t, "yes")})
	if args["with_runs"] != true {
		t.Errorf("the flag rides the wire as %v, want true", args["with_runs"])
	}
	encoded, err := json.Marshal(WorkflowListRequest{WithRuns: boolFlagOf(t, "yes")})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"with_runs":true}` {
		t.Errorf("the row encodes as %s, want the boolean", encoded)
	}
}

// TestLimitValueTellsAbsentFromZero says a limit that was not asked for is
// not the same answer as a limit of zero.
func TestLimitValueTellsAbsentFromZero(t *testing.T) {
	if got := LimitValue(Limit{}, 25); got != 25 {
		t.Errorf("an absent limit reads as %d, want the fallback 25", got)
	}
	var zero Limit
	if err := json.Unmarshal([]byte(`0`), &zero); err != nil {
		t.Fatal(err)
	}
	if got := LimitValue(zero, 25); got != 0 {
		t.Errorf("a limit of zero reads as %d, want 0", got)
	}
	var asked Limit
	if err := json.Unmarshal([]byte(`1.9`), &asked); err != nil {
		t.Fatal(err)
	}
	if got := LimitValue(asked, 25); got != 1 {
		t.Errorf("a limit of 1.9 reads as %d, want 1", got)
	}
}

// TestTheObjectIsTheObjectItCarries says an object field carries its map and
// nothing else.
func TestTheObjectIsTheObjectItCarries(t *testing.T) {
	var object Object
	if err := json.Unmarshal([]byte(`{"nodes":[{"id":"start"}]}`), &object); err != nil {
		t.Fatal(err)
	}
	if len(object.Map()) != 1 {
		t.Errorf("the object carries %v", object.Map())
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"nodes":[{"id":"start"}]}` {
		t.Errorf("the object encodes as %s", encoded)
	}
	if err := json.Unmarshal([]byte(`null`), &object); err == nil {
		t.Error("a null must not read as an object")
	}
}

// boolFlagOf reads a flag from the JSON value it arrives as.
func boolFlagOf(t *testing.T, value any) BoolFlag {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var flag BoolFlag
	if err := json.Unmarshal(encoded, &flag); err != nil {
		t.Fatalf("the flag %s does not decode: %v", encoded, err)
	}
	return flag
}
