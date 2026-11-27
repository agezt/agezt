// SPDX-License-Identifier: MIT

package opapi

// The request rows and the arguments they carry.
//
// A request row is one declaration with three readers — the registry derives
// the schema from it, the application layer decodes it, the console builds its
// arguments from it — so the row's field names are the only spelling of them.

import (
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
			t.Errorf("the %s request row names no field", binding.Operation)
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
