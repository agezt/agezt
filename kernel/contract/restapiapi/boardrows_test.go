// SPDX-License-Identifier: MIT

package restapiapi

// The board's mail row has one home.
//
// The REST contract names the mail row the SDKs speak, and the daemon serves
// the board's wire row — which lives in the BOARD contract
// (kernel/contract/boardapi), because three surfaces serve it and none of
// them may spell its field names a second time.
//
// The two are related by the contract's own rule: a RESPONSE row is the STABLE
// SUBSET of the payload. The daemon may add a field the SDKs do not name
// (`acked_by` is one — the server may add fields to a response), so the
// subset direction is the one that binds:
//
//   - every field the contract names must be on the wire row — a renamed wire
//     field would leave the SDKs reading a field the daemon no longer sends;
//   - the wire row may carry MORE — that is the rule, not a drift.
//
// The types are checked too: a field the contract calls a string and the wire
// carries as a number is a field the SDK would misread.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/boardapi"
)

// wireNames reads the wire names of a struct's fields.
func wireNames(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		out[name] = t.Field(i).Type
	}
	return out
}

// goTypesFor is the Go types a wire type may be read as.
func goTypesFor(t Type) []reflect.Type {
	switch t {
	case TypeString:
		return []reflect.Type{reflect.TypeOf("")}
	case TypeBool:
		return []reflect.Type{reflect.TypeOf(false)}
	case TypeInt:
		return []reflect.Type{
			reflect.TypeOf(int(0)), reflect.TypeOf(int64(0)), reflect.TypeOf(uint64(0)),
		}
	case TypeStringSlice:
		return []reflect.Type{reflect.TypeOf([]string{})}
	case TypeAnySlice:
		return []reflect.Type{reflect.TypeOf([]any{})}
	case TypeAnyObject:
		return []reflect.Type{reflect.TypeOf(map[string]any{})}
	case TypeIntMap:
		return []reflect.Type{reflect.TypeOf(map[string]int{})}
	default:
		return nil
	}
}

// typeAgrees reports whether the wire's Go type is one the contract's type
// allows.
func typeAgrees(wire Type, got reflect.Type) bool {
	for _, want := range goTypesFor(wire) {
		if got == want {
			return true
		}
	}
	return false
}

// TestTheMailRowIsAStableSubsetOfTheBoardsWireRow is the ratchet: every field
// the contract's mail row names must be a field of the board's wire row, under
// the same name and a type the contract allows.
func TestTheMailRowIsAStableSubsetOfTheBoardsWireRow(t *testing.T) {
	row := rowByName(t, RowMail)
	wire := wireNames(reflect.TypeOf(boardapi.Mail{}))
	for _, f := range row.Fields {
		got, ok := wire[f.Name]
		if !ok {
			t.Errorf("the contract's %q row names %q, which the board's wire row does not carry", row.Name, f.Name)
			continue
		}
		if !typeAgrees(f.Type, got) {
			t.Errorf("the contract's %q row names %q as %q, the board's wire row carries it as %s",
				row.Name, f.Name, f.Type, got)
		}
	}
}
